package backend

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modNtdll          = windows.NewLazySystemDLL("ntdll.dll")
	procNtSuspendProc = modNtdll.NewProc("NtSuspendProcess")
	procNtResumeProc  = modNtdll.NewProc("NtResumeProcess")

	modUser32           = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows     = modUser32.NewProc("EnumWindows")
	procGetWindowThread = modUser32.NewProc("GetWindowThreadProcessId")
	procPostMessage     = modUser32.NewProc("PostMessageW")
)

const (
	wmClose = 0x0010
)

var (
	suspendedPIDsMu sync.Mutex
	suspendedPIDs   = map[uint32]bool{}
)

// IsProcessSuspended checks if a PID was suspended by WinBGM.
func IsProcessSuspended(pid uint32) bool {
	suspendedPIDsMu.Lock()
	defer suspendedPIDsMu.Unlock()
	return suspendedPIDs[pid]
}

// guardProtectedPID enforces Go-side protection so critical Windows, security, and self processes
// can never be suspended, closed, or terminated regardless of what the UI sends.
func guardProtectedPID(pid uint32) error {
	if pid == 0 || pid == 4 {
		return errors.New("refusing action on critical Windows kernel/idle process")
	}
	p := imagePath(pid)
	name := filepath.Base(p)
	if isProtectedProcess(name, pid) {
		return fmt.Errorf("refusing action on protected system or security process (%s)", name)
	}
	return nil
}

// SuspendProcess freezes all threads in the process via NtSuspendProcess.
func SuspendProcess(pid uint32) error {
	if err := guardProtectedPID(pid); err != nil {
		return err
	}
	h, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, pid)
	if err != nil {
		return fmt.Errorf("failed to open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	r, _, _ := procNtSuspendProc.Call(uintptr(h))
	if r != 0 {
		return fmt.Errorf("NtSuspendProcess failed with NTSTATUS: 0x%X", r)
	}

	suspendedPIDsMu.Lock()
	suspendedPIDs[pid] = true
	suspendedPIDsMu.Unlock()
	return nil
}

// ResumeProcess unfreezes a suspended process via NtResumeProcess.
func ResumeProcess(pid uint32) error {
	if err := guardProtectedPID(pid); err != nil {
		return err
	}
	h, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, pid)
	if err != nil {
		return fmt.Errorf("failed to open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	r, _, _ := procNtResumeProc.Call(uintptr(h))
	if r != 0 {
		return fmt.Errorf("NtResumeProcess failed with NTSTATUS: 0x%X", r)
	}

	suspendedPIDsMu.Lock()
	delete(suspendedPIDs, pid)
	suspendedPIDsMu.Unlock()
	return nil
}

// GracefulCloseProcess posts WM_CLOSE to all top-level windows of the process and waits up to 2 seconds.
func GracefulCloseProcess(pid uint32) error {
	if err := guardProtectedPID(pid); err != nil {
		return err
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("failed to open process: %w", err)
	}
	defer windows.CloseHandle(h)

	var foundWindows []windows.HWND
	cb := windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var wndPID uint32
		procGetWindowThread.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wndPID)))
		if wndPID == pid {
			foundWindows = append(foundWindows, hwnd)
		}
		return 1
	})

	procEnumWindows.Call(cb, 0)

	if len(foundWindows) > 0 {
		for _, hwnd := range foundWindows {
			procPostMessage.Call(uintptr(hwnd), wmClose, 0, 0)
		}
		// Wait up to 2 seconds for clean exit
		event, _ := windows.WaitForSingleObject(h, 2000)
		if event == windows.WAIT_OBJECT_0 {
			return nil
		}
		return errors.New("process did not exit within timeout; forceful termination required")
	}

	return errors.New("no UI windows found to receive graceful close")
}

// KillProcess forcefully terminates a process.
func KillProcess(pid uint32) error {
	if err := guardProtectedPID(pid); err != nil {
		return err
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return fmt.Errorf("cannot open process to terminate: %w", err)
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}
