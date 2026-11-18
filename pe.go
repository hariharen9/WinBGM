package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type exeInfo struct {
	Company, Product, Sig string
	Exists                bool
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
)

func fileExists(p string) bool {
	if !strings.ContainsAny(p, `\/`) {
		_, err := exec.LookPath(p)
		return err == nil
	}
	_, err := os.Stat(p)
	return err == nil
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
// Files signed via Windows catalogs (many OS files) report "unsigned" here; those live in \Windows and are treated as system.
func verifySig(path string) string {
	pp, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "unsigned"
	}
	fi := &winTrustFileInfo{pcwszFilePath: pp}
	fi.cbStruct = uint32(unsafe.Sizeof(*fi))
	d := &winTrustData{
		dwUIChoice:          2,      // WTD_UI_NONE
		fdwRevocationChecks: 0,      // WTD_REVOKE_NONE
		dwUnionChoice:       1,      // WTD_CHOICE_FILE
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
