package backend

import (
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32Tray          = windows.NewLazySystemDLL("user32.dll")
	modShell32Tray         = windows.NewLazySystemDLL("shell32.dll")
	modKernel32Tray        = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClassExW   = modUser32Tray.NewProc("RegisterClassExW")
	procCreateWindowExW    = modUser32Tray.NewProc("CreateWindowExW")
	procDefWindowProcW     = modUser32Tray.NewProc("DefWindowProcW")
	procGetMessageW        = modUser32Tray.NewProc("GetMessageW")
	procTranslateMessage   = modUser32Tray.NewProc("TranslateMessage")
	procDispatchMessageW   = modUser32Tray.NewProc("DispatchMessageW")
	procPostMessageW       = modUser32Tray.NewProc("PostMessageW")
	procPostQuitMessage    = modUser32Tray.NewProc("PostQuitMessage")
	procDestroyWindow      = modUser32Tray.NewProc("DestroyWindow")
	procCreatePopupMenu    = modUser32Tray.NewProc("CreatePopupMenu")
	procAppendMenuW        = modUser32Tray.NewProc("AppendMenuW")
	procTrackPopupMenu     = modUser32Tray.NewProc("TrackPopupMenu")
	procDestroyMenu        = modUser32Tray.NewProc("DestroyMenu")
	procGetCursorPos       = modUser32Tray.NewProc("GetCursorPos")
	procSetForegroundWin   = modUser32Tray.NewProc("SetForegroundWindow")
	procLoadIconW          = modUser32Tray.NewProc("LoadIconW")
	procShellNotifyIconW   = modShell32Tray.NewProc("Shell_NotifyIconW")
	procExtractIconW       = modShell32Tray.NewProc("ExtractIconW")
	procGetModuleHandleW   = modKernel32Tray.NewProc("GetModuleHandleW")
)

const (
	wmDestroy       = 0x0002
	wmCommand       = 0x0111
	wmUser          = 0x0400
	wmTrayIcon      = wmUser + 42
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205

	nimAdd    = 0x00000000
	nimDelete = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	mfString    = 0x00000000
	mfSeparator = 0x00000800
	tpmRightBtn = 0x0002
	tpmBottom   = 0x0020

	cmdTrayOpen = 1001
	cmdTrayScan = 1002
	cmdTrayQuit = 1003
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type notifyIconDataW struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
}

type point struct {
	x, y int32
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

var (
	trayMu      sync.Mutex
	trayHwnd    uintptr
	trayRunning bool
	trayOnOpen  func()
	trayOnScan  func()
	trayOnQuit  func()
)

// StartTray creates a native Windows system tray icon with context menu.
func StartTray(onOpen, onScan, onQuit func()) {
	trayMu.Lock()
	if trayRunning {
		trayMu.Unlock()
		return
	}
	trayRunning = true
	trayOnOpen = onOpen
	trayOnScan = onScan
	trayOnQuit = onQuit
	trayMu.Unlock()

	go runTrayLoop()
}

// StopTray removes the system tray icon and exits the message loop.
func StopTray() {
	trayMu.Lock()
	hwnd := trayHwnd
	trayRunning = false
	trayMu.Unlock()

	if hwnd != 0 {
		nid := notifyIconDataW{
			cbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
			hWnd:   hwnd,
			uID:    1,
		}
		procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
		procPostMessageW.Call(hwnd, wmDestroy, 0, 0)
	}
}

func trayWndProc(hwnd, uMsg, wParam, lParam uintptr) uintptr {
	switch uMsg {
	case wmTrayIcon:
		switch lParam {
		case wmLButtonUp, wmLButtonDblClk:
			if trayOnOpen != nil {
				go trayOnOpen()
			}
		case wmRButtonUp:
			showTrayMenu(hwnd)
		}
		return 0
	case wmCommand:
		switch wParam & 0xFFFF {
		case cmdTrayOpen:
			if trayOnOpen != nil {
				go trayOnOpen()
			}
		case cmdTrayScan:
			if trayOnScan != nil {
				go trayOnScan()
			}
		case cmdTrayQuit:
			if trayOnQuit != nil {
				go trayOnQuit()
			}
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uMsg, wParam, lParam)
	return ret
}

func showTrayMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	openTxt, _ := windows.UTF16PtrFromString("Open WinBGM")
	scanTxt, _ := windows.UTF16PtrFromString("Quick Scan Now")
	quitTxt, _ := windows.UTF16PtrFromString("Quit WinBGM")

	procAppendMenuW.Call(hMenu, mfString, cmdTrayOpen, uintptr(unsafe.Pointer(openTxt)))
	procAppendMenuW.Call(hMenu, mfString, cmdTrayScan, uintptr(unsafe.Pointer(scanTxt)))
	procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	procAppendMenuW.Call(hMenu, mfString, cmdTrayQuit, uintptr(unsafe.Pointer(quitTxt)))

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWin.Call(hwnd)
	procTrackPopupMenu.Call(hMenu, tpmRightBtn|tpmBottom, uintptr(pt.x), uintptr(pt.y), 0, hwnd, 0)
	procPostMessageW.Call(hwnd, 0, 0, 0)
}

func runTrayLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := windows.UTF16PtrFromString("WinBGMTrayClass")

	wndProc := windows.NewCallback(trayWndProc)
	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProc,
		hInstance:     hInst,
		lpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0, 0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return
	}

	trayMu.Lock()
	trayHwnd = hwnd
	trayMu.Unlock()

	// Extract icon from current executable, fallback to default application icon
	var hIcon uintptr
	if exe, err := os.Executable(); err == nil {
		if exePtr, err := windows.UTF16PtrFromString(exe); err == nil {
			hIcon, _, _ = procExtractIconW.Call(hInst, uintptr(unsafe.Pointer(exePtr)), 0)
		}
	}
	if hIcon == 0 || hIcon == 1 {
		hIcon, _, _ = procLoadIconW.Call(0, 32512) // IDI_APPLICATION
	}

	var nid notifyIconDataW
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hwnd
	nid.uID = 1
	nid.uFlags = nifMessage | nifIcon | nifTip
	nid.uCallbackMessage = wmTrayIcon
	nid.hIcon = hIcon
	tip, _ := windows.UTF16FromString("WinBGM — Background Guard Active")
	copy(nid.szTip[:], tip)

	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
