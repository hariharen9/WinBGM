package backend

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel32Instance = windows.NewLazySystemDLL("kernel32.dll")
	procCreateMutexW    = modKernel32Instance.NewProc("CreateMutexW")
	procCreateEventW    = modKernel32Instance.NewProc("CreateEventW")

	modUser32Instance        = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW          = modUser32Instance.NewProc("FindWindowW")
	procShowWindowInst       = modUser32Instance.NewProc("ShowWindow")
	procSetForegroundWinInst = modUser32Instance.NewProc("SetForegroundWindow")
	procIsIconicInst         = modUser32Instance.NewProc("IsIconic")
)

const (
	appMutexName     = `Local\WinBGM.SingleInstance`
	appQuitEventName = `Local\WinBGM.QuitEvent`
	swRestore        = 9
)

var singleInstanceHandle windows.Handle

// AcquireSingleInstanceLock tries to become the only running WinBGM.
// It returns false when another instance already holds the lock.
// The lock lives for the process lifetime; no explicit release is needed.
func AcquireSingleInstanceLock() bool {
	name, err := windows.UTF16PtrFromString(appMutexName)
	if err != nil {
		return true // Fail open: never block the app over a naming error.
	}
	lastErr := syscall.Errno(0)
	h, _, callErr := procCreateMutexW.Call(
		0, // default security attributes
		1, // initial owner
		uintptr(unsafe.Pointer(name)),
	)
	if h == 0 {
		return true
	}
	if errors_AsErrno(callErr, &lastErr) && lastErr == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(windows.Handle(h))
		return false
	}
	singleInstanceHandle = windows.Handle(h)
	return true
}

func errors_AsErrno(callErr error, target *syscall.Errno) bool {
	if callErr == nil {
		return false
	}
	if e, ok := callErr.(syscall.Errno); ok {
		*target = e
		return true
	}
	return false
}

// FocusExistingWindow brings the already-running WinBGM to the foreground.
func FocusExistingWindow() {
	title, err := windows.UTF16PtrFromString("WinBGM")
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return
	}
	iconic, _, _ := procIsIconicInst.Call(hwnd)
	if iconic != 0 {
		procShowWindowInst.Call(hwnd, swRestore)
	}
	procSetForegroundWinInst.Call(hwnd)
}

// StartQuitWatcher creates (or opens) the named quit event and returns a
// channel that closes when another process signals it (the installer's
// `WinBGM.exe --quit` runs before uninstalling).
func StartQuitWatcher() <-chan struct{} {
	ch := make(chan struct{})
	name, err := windows.UTF16PtrFromString(appQuitEventName)
	if err != nil {
		return ch
	}
	h, _, _ := procCreateEventW.Call(
		0, // default security attributes
		1, // manual reset, so a missed waiter can still observe the signal
		0, // initially unsignaled
		uintptr(unsafe.Pointer(name)),
	)
	if h == 0 {
		return ch
	}
	// Clear any stale signal left over from a previous --quit so a freshly
	// started instance never mistakes it for a new quit request.
	_ = windows.ResetEvent(windows.Handle(h))
	go func() {
		defer windows.CloseHandle(windows.Handle(h))
		s, err := windows.WaitForSingleObject(windows.Handle(h), windows.INFINITE)
		if err == nil && s == windows.WAIT_OBJECT_0 {
			close(ch)
		}
	}()
	return ch
}

// SignalQuit signals the quit event of a running WinBGM instance, if any.
func SignalQuit() {
	name, err := windows.UTF16PtrFromString(appQuitEventName)
	if err != nil {
		return
	}
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return // Nothing running.
	}
	defer windows.CloseHandle(h)
	_ = windows.SetEvent(h)
}
