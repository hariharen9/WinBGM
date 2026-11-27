package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type exeInfo struct {
	Company, Product, Sig, Signer, SHA256 string
	Expired                               bool
	Exists                                bool
}

var (
	infoMu    sync.Mutex
	infoCache = map[string]exeInfo{}

	modVersion = windows.NewLazySystemDLL("version.dll")
	pVerSize   = modVersion.NewProc("GetFileVersionInfoSizeW")
	pVerInfo   = modVersion.NewProc("GetFileVersionInfoW")
	pVerQuery  = modVersion.NewProc("VerQueryValueW")

	modWintrust = windows.NewLazySystemDLL("wintrust.dll")
	pWinTrust   = modWintrust.NewProc("WinVerifyTrust")

	procCryptCATAdminAcquireContext      = modWintrust.NewProc("CryptCATAdminAcquireContext")
	procCryptCATAdminCalcHashFromFileH   = modWintrust.NewProc("CryptCATAdminCalcHashFromFileHandle")
	procCryptCATAdminEnumCatalogFromHash = modWintrust.NewProc("CryptCATAdminEnumCatalogFromHash")
	procCryptCATAdminReleaseCatalogCtx   = modWintrust.NewProc("CryptCATAdminReleaseCatalogContext")
	procCryptCATAdminReleaseCtx          = modWintrust.NewProc("CryptCATAdminReleaseContext")

	modCrypt32                      = windows.NewLazySystemDLL("crypt32.dll")
	procCryptQueryObject            = modCrypt32.NewProc("CryptQueryObject")
	procCryptMsgClose               = modCrypt32.NewProc("CryptMsgClose")
	procCertEnumCertificatesInStore = modCrypt32.NewProc("CertEnumCertificatesInStore")
)

const (
	certQueryObjectFile                  = 0x00000001
	certQueryContentFlagPKCS7SignedEmbed = 1 << 10
	certQueryFormatFlagAll               = 0x0000000E
)

func fileExists(p string) bool {
	if !strings.ContainsAny(p, `\/`) {
		_, err := exec.LookPath(p)
		return err == nil
	}
	_, err := os.Stat(p)
	return err == nil
}

func calcSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	// Read up to 80MB to maintain responsive scanning on very large binaries
	if _, err := io.Copy(h, io.LimitReader(f, 80*1024*1024)); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func lookupInfo(path string) exeInfo {
	key := strings.ToLower(path)
	infoMu.Lock()
	if v, ok := infoCache[key]; ok {
		infoMu.Unlock()
		return v
	}
	infoMu.Unlock()

	var e exeInfo
	if fileExists(path) {
		e.Exists = true
		e.Company, e.Product = versionStrings(path)
		e.Sig = verifySig(path)
		if e.Sig == "valid" {
			e.Signer, e.Expired = getCertSigner(path)
		} else if verifyCatalogSig(path) {
			e.Sig = "catalog"
			e.Signer = "Microsoft Windows"
		}
		e.SHA256 = calcSHA256(path)
	}
	infoMu.Lock()
	infoCache[key] = e
	infoMu.Unlock()
	return e
}

func versionStrings(path string) (company, product string) {
	pp, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	size, _, _ := pVerSize.Call(uintptr(unsafe.Pointer(pp)), 0)
	if size == 0 {
		return
	}
	buf := make([]byte, size)
	if r, _, _ := pVerInfo.Call(uintptr(unsafe.Pointer(pp)), 0, size, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return
	}
	query := func(sub string) (unsafe.Pointer, uint32) {
		sp, _ := windows.UTF16PtrFromString(sub)
		var ptr unsafe.Pointer
		var n uint32
		r, _, _ := pVerQuery.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(sp)),
			uintptr(unsafe.Pointer(&ptr)), uintptr(unsafe.Pointer(&n)))
		if r == 0 {
			return nil, 0
		}
		return ptr, n
	}
	tp, tn := query(`\VarFileInfo\Translation`)
	if tp == nil || tn < 4 {
		return
	}
	lang := *(*uint16)(tp)
	cp := *(*uint16)(unsafe.Add(tp, 2))
	get := func(name string) string {
		p, n := query(fmt.Sprintf(`\StringFileInfo\%04x%04x\%s`, lang, cp, name))
		if p == nil || n == 0 {
			return ""
		}
		return strings.TrimSpace(windows.UTF16PtrToString((*uint16)(p)))
	}
	return get("CompanyName"), get("ProductName")
}

type winTrustFileInfo struct {
	cbStruct       uint32
	pcwszFilePath  *uint16
	hFile          windows.Handle
	pgKnownSubject *windows.GUID
}

type winTrustData struct {
	cbStruct            uint32
	pPolicyCallbackData uintptr
	pSIPClientData      uintptr
	dwUIChoice          uint32
	fdwRevocationChecks uint32
	dwUnionChoice       uint32
	pFile               *winTrustFileInfo
	dwStateAction       uint32
	hWVTStateData       windows.Handle
	pwszURLReference    *uint16
	dwProvFlags         uint32
	dwUIContext         uint32
	pSignatureSettings  uintptr
}

// WINTRUST_ACTION_GENERIC_VERIFY_V2
var actionGenericVerifyV2 = windows.GUID{Data1: 0x00AAC56B, Data2: 0xCD44, Data3: 0x11D0,
	Data4: [8]byte{0x8C, 0xC2, 0x00, 0xC0, 0x4F, 0xC2, 0x95, 0xEE}}

// verifySig checks an embedded Authenticode signature. No UI, no network.
func verifySig(path string) string {
	pp, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "unsigned"
	}
	fi := &winTrustFileInfo{pcwszFilePath: pp}
	fi.cbStruct = uint32(unsafe.Sizeof(*fi))
	d := &winTrustData{
		dwUIChoice:          2, // WTD_UI_NONE
		fdwRevocationChecks: 0, // WTD_REVOKE_NONE
		dwUnionChoice:       1, // WTD_CHOICE_FILE
		pFile:               fi,
		dwStateAction:       1,      // WTD_STATEACTION_VERIFY
		dwProvFlags:         0x1000, // WTD_CACHE_ONLY_URL_RETRIEVAL
	}
	d.cbStruct = uint32(unsafe.Sizeof(*d))
	invalid := ^uintptr(0)
	r, _, _ := pWinTrust.Call(invalid, uintptr(unsafe.Pointer(&actionGenericVerifyV2)), uintptr(unsafe.Pointer(d)))
	d.dwStateAction = 2 // WTD_STATEACTION_CLOSE
	pWinTrust.Call(invalid, uintptr(unsafe.Pointer(&actionGenericVerifyV2)), uintptr(unsafe.Pointer(d)))
	if r == 0 {
		return "valid"
	}
	return "unsigned"
}

// verifyCatalogSig checks Windows catalog file signature (.cat files under CatRoot).
func verifyCatalogSig(path string) bool {
	var hCatAdmin windows.Handle
	r, _, _ := procCryptCATAdminAcquireContext.Call(uintptr(unsafe.Pointer(&hCatAdmin)), 0, 0)
	if r == 0 || hCatAdmin == 0 {
		return false
	}
	defer procCryptCATAdminReleaseCtx.Call(uintptr(hCatAdmin), 0)

	pwsz, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	hFile, err := windows.CreateFile(pwsz, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(hFile)

	var cbHash uint32
	r, _, _ = procCryptCATAdminCalcHashFromFileH.Call(uintptr(hFile), uintptr(unsafe.Pointer(&cbHash)), 0, 0)
	if cbHash == 0 {
		return false
	}

	hashBuf := make([]byte, cbHash)
	r, _, _ = procCryptCATAdminCalcHashFromFileH.Call(uintptr(hFile), uintptr(unsafe.Pointer(&cbHash)), uintptr(unsafe.Pointer(&hashBuf[0])), 0)
	if r == 0 {
		return false
	}

	hCatInfo, _, _ := procCryptCATAdminEnumCatalogFromHash.Call(
		uintptr(hCatAdmin),
		uintptr(unsafe.Pointer(&hashBuf[0])),
		uintptr(cbHash),
		0,
		0,
	)
	if hCatInfo != 0 {
		procCryptCATAdminReleaseCatalogCtx.Call(uintptr(hCatAdmin), hCatInfo, 0)
		return true
	}
	return false
}

// getCertSigner extracts the certificate subject display name and checks validity dates.
func getCertSigner(filePath string) (signer string, expired bool) {
	pwsz, err := windows.UTF16PtrFromString(filePath)
	if err != nil {
		return "", false
	}

	var encoding, contentType, formatType uint32
	var hStore, hMsg windows.Handle

	r, _, _ := procCryptQueryObject.Call(
		certQueryObjectFile,
		uintptr(unsafe.Pointer(pwsz)),
		certQueryContentFlagPKCS7SignedEmbed,
		certQueryFormatFlagAll,
		0,
		uintptr(unsafe.Pointer(&encoding)),
		uintptr(unsafe.Pointer(&contentType)),
		uintptr(unsafe.Pointer(&formatType)),
		uintptr(unsafe.Pointer(&hStore)),
		uintptr(unsafe.Pointer(&hMsg)),
		0,
	)
	if r == 0 || hStore == 0 {
		return "", false
	}
	defer func() {
		windows.CertCloseStore(hStore, 0)
		if hMsg != 0 {
			procCryptMsgClose.Call(uintptr(hMsg))
		}
	}()

	var pCertCtx *windows.CertContext
	for {
		pCertCtxRaw, _, _ := procCertEnumCertificatesInStore.Call(uintptr(hStore), uintptr(unsafe.Pointer(pCertCtx)))
		if pCertCtxRaw == 0 {
			break
		}
		pCertCtx = (*windows.CertContext)(unsafe.Pointer(pCertCtxRaw))
		var buf [256]uint16
		n := windows.CertGetNameString(pCertCtx, windows.CERT_NAME_SIMPLE_DISPLAY_TYPE, 0, nil, &buf[0], uint32(len(buf)))
		if n > 1 {
			name := windows.UTF16ToString(buf[:n])
			if signer == "" {
				signer = name
				if pCertCtx.CertInfo != nil {
					notAfter := time.Unix(0, pCertCtx.CertInfo.NotAfter.Nanoseconds())
					if time.Now().After(notAfter) {
						expired = true
					}
				}
			}
		}
	}
	return signer, expired
}
