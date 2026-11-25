package backend

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modShell32Icon         = windows.NewLazySystemDLL("shell32.dll")
	modUser32Icon          = windows.NewLazySystemDLL("user32.dll")
	modGdi32Icon           = windows.NewLazySystemDLL("gdi32.dll")
	procSHGetFileInfoW     = modShell32Icon.NewProc("SHGetFileInfoW")
	procExtractIconExW     = modShell32Icon.NewProc("ExtractIconExW")
	procGetIconInfo        = modUser32Icon.NewProc("GetIconInfo")
	procDestroyIcon        = modUser32Icon.NewProc("DestroyIcon")
	procCreateCompatibleDC = modGdi32Icon.NewProc("CreateCompatibleDC")
	procDeleteDC           = modGdi32Icon.NewProc("DeleteDC")
	procGetDIBits          = modGdi32Icon.NewProc("GetDIBits")
	procDeleteObject       = modGdi32Icon.NewProc("DeleteObject")
)

const (
	shgfiIcon      = 0x000000100
	shgfiLargeIcon = 0x000000000
	biRGB          = 0
	dibRGBColors   = 0
)

type shFileInfoW struct {
	hIcon         uintptr
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

var (
	iconCacheMu sync.RWMutex
	iconCache   = make(map[string]string) // lowercase rawPath -> "data:image/png;base64,..." ("" = no icon)
)

// parseIconSpec splits a DisplayIcon string like `"C:\App\app.exe",0` into path and icon index.
func parseIconSpec(spec string) (string, int) {
	s := strings.TrimSpace(spec)
	if s == "" {
		return "", 0
	}
	idx := 0
	if comma := strings.LastIndex(s, ","); comma > 0 {
		tail := strings.TrimSpace(s[comma+1:])
		if n, err := strconv.Atoi(tail); err == nil {
			idx = n
			s = s[:comma]
		}
	}
	s = strings.Trim(s, `" `)
	return s, idx
}

func extractHIcon(rawSpec string) uintptr {
	p, idx := parseIconSpec(rawSpec)
	if p == "" {
		return 0
	}
	p = ExpandPath(p)
	if _, err := os.Stat(p); err != nil {
		return 0
	}

	u16, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return 0
	}

	ext := strings.ToLower(filepath.Ext(p))
	// For binaries and icon files, use ExtractIconExW so binaries without an embedded icon
	// return 0 instead of SHGetFileInfoW's generic Windows placeholder box icon.
	if ext == ".exe" || ext == ".dll" || ext == ".ico" || ext == ".cpl" {
		var hLarge uintptr
		ret, _, _ := procExtractIconExW.Call(
			uintptr(unsafe.Pointer(u16)),
			uintptr(idx),
			uintptr(unsafe.Pointer(&hLarge)),
			0,
			1,
		)
		if ret > 0 && hLarge != 0 {
			return hLarge
		}
		return 0
	}

	var shfi shFileInfoW
	ret, _, _ := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(u16)),
		0,
		uintptr(unsafe.Pointer(&shfi)),
		unsafe.Sizeof(shfi),
		shgfiIcon|shgfiLargeIcon,
	)
	if ret != 0 && shfi.hIcon != 0 {
		return shfi.hIcon
	}
	return 0
}

func hiconToPNGDataURI(hIcon uintptr) string {
	if hIcon == 0 {
		return ""
	}
	var ii iconInfo
	if ret, _, _ := procGetIconInfo.Call(hIcon, uintptr(unsafe.Pointer(&ii))); ret == 0 {
		return ""
	}
	if ii.hbmMask != 0 {
		defer procDeleteObject.Call(ii.hbmMask)
	}
	if ii.hbmColor != 0 {
		defer procDeleteObject.Call(ii.hbmColor)
	}

	const w, h = 32, 32
	hdc, _, _ := procCreateCompatibleDC.Call(0)
	if hdc == 0 {
		return ""
	}
	defer procDeleteDC.Call(hdc)

	var bi bitmapInfoHeader
	bi.biSize = uint32(unsafe.Sizeof(bi))
	bi.biWidth = w
	bi.biHeight = -h // top-down DIB
	bi.biPlanes = 1
	bi.biBitCount = 32
	bi.biCompression = biRGB

	pixels := make([]byte, w*h*4)
	if ii.hbmColor != 0 {
		ret, _, _ := procGetDIBits.Call(
			hdc,
			ii.hbmColor,
			0, h,
			uintptr(unsafe.Pointer(&pixels[0])),
			uintptr(unsafe.Pointer(&bi)),
			dibRGBColors,
		)
		if ret == 0 {
			return ""
		}
	} else if ii.hbmMask != 0 {
		// Monochrome icon fallback
		ret, _, _ := procGetDIBits.Call(
			hdc,
			ii.hbmMask,
			0, h,
			uintptr(unsafe.Pointer(&pixels[0])),
			uintptr(unsafe.Pointer(&bi)),
			dibRGBColors,
		)
		if ret == 0 {
			return ""
		}
	} else {
		return ""
	}

	// Check if color bitmap actually has non-zero alpha channel; if all alpha==0, apply hbmMask
	hasAlpha := false
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 0 {
			hasAlpha = true
			break
		}
	}

	var maskPixels []byte
	if !hasAlpha && ii.hbmMask != 0 {
		maskPixels = make([]byte, w*h*4)
		procGetDIBits.Call(
			hdc,
			ii.hbmMask,
			0, h,
			uintptr(unsafe.Pointer(&maskPixels[0])),
			uintptr(unsafe.Pointer(&bi)),
			dibRGBColors,
		)
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		off := i * 4
		b := pixels[off]
		g := pixels[off+1]
		r := pixels[off+2]
		a := pixels[off+3]
		if !hasAlpha && len(maskPixels) == len(pixels) {
			// In Win32 AND mask, 0 = opaque, 255 = transparent
			if maskPixels[off] == 0 {
				a = 255
			} else {
				a = 0
			}
		}
		img.Pix[off] = r
		img.Pix[off+1] = g
		img.Pix[off+2] = b
		img.Pix[off+3] = a
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// GetAppIcons extracts 32x32 PNG data URIs for a batch of executable/icon paths, using an in-memory cache.
func GetAppIcons(paths []string) map[string]string {
	out := make(map[string]string, len(paths))
	for _, raw := range paths {
		if raw == "" {
			continue
		}
		key := strings.ToLower(raw)

		iconCacheMu.RLock()
		cached, ok := iconCache[key]
		iconCacheMu.RUnlock()

		if ok {
			if cached != "" {
				out[key] = cached
			}
			continue
		}

		hIcon := extractHIcon(raw)
		dataURI := ""
		if hIcon != 0 {
			dataURI = hiconToPNGDataURI(hIcon)
			procDestroyIcon.Call(hIcon)
		}

		iconCacheMu.Lock()
		iconCache[key] = dataURI
		iconCacheMu.Unlock()

		if dataURI != "" {
			out[key] = dataURI
		}
	}
	return out
}
