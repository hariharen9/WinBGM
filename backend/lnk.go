package backend

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// MS-SHLLINK flags
const (
	lnkHasLinkTargetIDList = 1 << 0
	lnkHasLinkInfo         = 1 << 1
	lnkHasName             = 1 << 2
	lnkHasRelativePath     = 1 << 3
	lnkHasWorkingDir       = 1 << 4
	lnkHasArguments        = 1 << 5
	lnkHasIconLocation     = 1 << 6
	lnkIsUnicode           = 1 << 7
)

var lnkCLSID = [16]byte{
	0x01, 0x14, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46,
}

// readLnk parses an MS-SHLLINK binary file directly in pure Go.
// Returns the executable/file target and any arguments.
func readLnk(path string) (target, args string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	if len(data) < 76 {
		return "", "", errors.New("lnk file too small")
	}

	headerSize := binary.LittleEndian.Uint32(data[0:4])
	if headerSize != 0x0000004C {
		return "", "", errors.New("invalid lnk header size")
	}

	if !bytes.Equal(data[4:20], lnkCLSID[:]) {
		return "", "", errors.New("invalid lnk clsid")
	}

	flags := binary.LittleEndian.Uint32(data[20:24])
	offset := 76

	// 1. LinkTargetIDList
	if flags&lnkHasLinkTargetIDList != 0 {
		if offset+2 > len(data) {
			return "", "", errors.New("truncated idlist")
		}
		idListSize := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
		offset += 2 + idListSize
	}

	// 2. LinkInfo
	if flags&lnkHasLinkInfo != 0 {
		if offset+4 <= len(data) {
			linkInfoSize := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
			if offset+linkInfoSize <= len(data) && linkInfoSize >= 28 {
				liHeaderSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
				localBasePathOffset := int(binary.LittleEndian.Uint32(data[offset+16 : offset+20]))
				commonPathSuffixOffset := int(binary.LittleEndian.Uint32(data[offset+24 : offset+28]))

				// Check for Unicode offsets if available
				var unicodeBasePathOffset, unicodeCommonPathSuffixOffset int
				if liHeaderSize >= 36 {
					unicodeBasePathOffset = int(binary.LittleEndian.Uint32(data[offset+28 : offset+32]))
					unicodeCommonPathSuffixOffset = int(binary.LittleEndian.Uint32(data[offset+32 : offset+36]))
				}

				if unicodeBasePathOffset > 0 && offset+unicodeBasePathOffset < len(data) {
					target = readNullTerminatedUTF16(data[offset+unicodeBasePathOffset:])
					if unicodeCommonPathSuffixOffset > 0 && offset+unicodeCommonPathSuffixOffset < len(data) {
						target += readNullTerminatedUTF16(data[offset+unicodeCommonPathSuffixOffset:])
					}
				} else if localBasePathOffset > 0 && offset+localBasePathOffset < len(data) {
					target = readNullTerminatedString(data[offset+localBasePathOffset:])
					if commonPathSuffixOffset > 0 && offset+commonPathSuffixOffset < len(data) {
						target += readNullTerminatedString(data[offset+commonPathSuffixOffset:])
					}
				}
			}
			offset += linkInfoSize
		}
	}

	// Helper for StringData
	isUnicode := flags&lnkIsUnicode != 0
	readStringData := func() string {
		if offset+2 > len(data) {
			return ""
		}
		cch := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
		offset += 2
		if isUnicode {
			byteCount := cch * 2
			if offset+byteCount > len(data) {
				return ""
			}
			s := decodeUTF16(data[offset : offset+byteCount])
			offset += byteCount
			return s
		}
		if offset+cch > len(data) {
			return ""
		}
		s := string(data[offset : offset+cch])
		offset += cch
		return s
	}

	if flags&lnkHasName != 0 {
		readStringData()
	}
	if flags&lnkHasRelativePath != 0 {
		rel := readStringData()
		if target == "" && rel != "" {
			target = filepath.Clean(filepath.Join(filepath.Dir(path), rel))
		}
	}
	if flags&lnkHasWorkingDir != 0 {
		readStringData()
	}
	if flags&lnkHasArguments != 0 {
		args = readStringData()
	}
	if flags&lnkHasIconLocation != 0 {
		readStringData()
	}

	// 3. ExtraData blocks (look for EnvironmentVariableDataBlock: 0xA0000001)
	for offset+8 <= len(data) {
		blockSize := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		if blockSize < 4 || offset+blockSize > len(data) {
			break
		}
		blockSig := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		if blockSig == 0xA0000001 && blockSize >= 268 {
			var envTarget string
			if blockSize >= 788 {
				envTarget = readNullTerminatedUTF16(data[offset+268 : offset+788])
			}
			if envTarget == "" {
				envTarget = readNullTerminatedString(data[offset+8 : offset+268])
			}
			if envTarget != "" {
				expanded := os.ExpandEnv(envTarget)
				if expanded != "" {
					target = expanded
				}
			}
		}
		offset += blockSize
	}

	target = strings.TrimSpace(target)
	args = strings.TrimSpace(args)
	return target, args, nil
}

func readNullTerminatedString(b []byte) string {
	idx := bytes.IndexByte(b, 0)
	if idx >= 0 {
		return string(b[:idx])
	}
	return string(b)
}

func readNullTerminatedUTF16(b []byte) string {
	var u16 []uint16
	for i := 0; i+1 < len(b); i += 2 {
		val := binary.LittleEndian.Uint16(b[i : i+2])
		if val == 0 {
			break
		}
		u16 = append(u16, val)
	}
	return string(utf16.Decode(u16))
}

func decodeUTF16(b []byte) string {
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = binary.LittleEndian.Uint16(b[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16))
}

var (
	modOle32             = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
)

// resolveShortcut parses a .lnk file using pure Go first, then falls back to COM IShellLinkW if needed.
func resolveShortcut(path string) (target, args string) {
	t, a, err := readLnk(path)
	if err == nil && t != "" {
		return t, a
	}
	// Fallback to COM IShellLinkW in-process
	comT, comA, comErr := resolveShortcutCOM(path)
	if comErr == nil && comT != "" {
		return comT, comA
	}
	return t, a
}

// resolveShortcutCOM uses Windows IShellLinkW in-process via ole32.dll with zero subprocess overhead.
func resolveShortcutCOM(lnkPath string) (target, args string, err error) {
	_ = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	defer ole.CoUninitialize()

	clsidShellLink := ole.NewGUID("{00021401-0000-0000-C000-000000000046}")
	iidShellLink := ole.NewGUID("{000214F9-0000-0000-C000-000000000046}")
	iidPersistFile := ole.NewGUID("{0000010B-0000-0000-C000-000000000046}")

	var slUnknown *ole.IUnknown
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(clsidShellLink)),
		0,
		1, // CLSCTX_INPROC_SERVER
		uintptr(unsafe.Pointer(iidShellLink)),
		uintptr(unsafe.Pointer(&slUnknown)),
	)
	if hr != 0 || slUnknown == nil {
		return "", "", syscall.Errno(hr)
	}
	defer slUnknown.Release()

	var pf *persistFile
	r, _, _ := syscall.SyscallN(slUnknown.VTable().QueryInterface, uintptr(unsafe.Pointer(slUnknown)), uintptr(unsafe.Pointer(iidPersistFile)), uintptr(unsafe.Pointer(&pf)))
	if r != 0 || pf == nil {
		return "", "", syscall.Errno(r)
	}
	defer pf.Release()

	pLnk, err := windows.UTF16PtrFromString(lnkPath)
	if err != nil {
		return "", "", err
	}
	if loadHr := pf.Load(pLnk, 0); loadHr != 0 {
		return "", "", syscall.Errno(loadHr)
	}

	sl := (*shellLinkW)(unsafe.Pointer(slUnknown))
	var targetBuf [windows.MAX_PATH]uint16
	var wfd windows.Win32finddata
	_ = sl.GetPath(&targetBuf[0], windows.MAX_PATH, &wfd, 0)
	target = windows.UTF16ToString(targetBuf[:])

	var argsBuf [1024]uint16
	_ = sl.GetArguments(&argsBuf[0], 1024)
	args = windows.UTF16ToString(argsBuf[:])

	return target, args, nil
}

type persistFileVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetClassID     uintptr
	IsDirty        uintptr
	Load           uintptr
	Save           uintptr
	SaveCompleted  uintptr
	GetCurFile     uintptr
}

type persistFile struct {
	vtbl *persistFileVtbl
}

func (p *persistFile) Release() uint32 {
	r, _, _ := syscall.SyscallN(p.vtbl.Release, uintptr(unsafe.Pointer(p)))
	return uint32(r)
}

func (p *persistFile) Load(pszFileName *uint16, dwMode uint32) uint32 {
	r, _, _ := syscall.SyscallN(p.vtbl.Load, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(pszFileName)), uintptr(dwMode))
	return uint32(r)
}

type shellLinkWVtbl struct {
	QueryInterface      uintptr
	AddRef              uintptr
	Release             uintptr
	GetPath             uintptr
	GetIDList           uintptr
	SetIDList           uintptr
	GetDescription      uintptr
	SetDescription      uintptr
	GetWorkingDirectory uintptr
	SetWorkingDirectory uintptr
	GetArguments        uintptr
	SetArguments        uintptr
	GetHotkey           uintptr
	SetHotkey           uintptr
	GetShowCmd          uintptr
	SetShowCmd          uintptr
	GetIconLocation     uintptr
	SetIconLocation     uintptr
	SetRelativePath     uintptr
	Resolve             uintptr
	SetPath             uintptr
}

type shellLinkW struct {
	vtbl *shellLinkWVtbl
}

func (s *shellLinkW) GetPath(pszFile *uint16, cchMaxPath int32, pfd *windows.Win32finddata, fFlags uint32) uint32 {
	r, _, _ := syscall.SyscallN(s.vtbl.GetPath, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(pszFile)), uintptr(cchMaxPath), uintptr(unsafe.Pointer(pfd)), uintptr(fFlags))
	return uint32(r)
}

func (s *shellLinkW) GetArguments(pszArgs *uint16, cchMaxPath int32) uint32 {
	r, _, _ := syscall.SyscallN(s.vtbl.GetArguments, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(pszArgs)), uintptr(cchMaxPath))
	return uint32(r)
}
