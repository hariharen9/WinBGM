package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modRstrtMgr             = windows.NewLazySystemDLL("rstrtmgr.dll")
	procRmStartSession      = modRstrtMgr.NewProc("RmStartSession")
	procRmRegisterResources = modRstrtMgr.NewProc("RmRegisterResources")
	procRmGetList           = modRstrtMgr.NewProc("RmGetList")
	procRmEndSession        = modRstrtMgr.NewProc("RmEndSession")
)

const (
	cchRmSessionKey = 32
	cchRmMaxAppName = 255
	cchRmMaxSvcName = 63
)

type rmUniqueProcess struct {
	dwProcessId      uint32
	processStartTime windows.Filetime
}

type rmProcessInfo struct {
	Process             rmUniqueProcess
	StrAppName          [cchRmMaxAppName + 1]uint16
	StrServiceShortName [cchRmMaxSvcName + 1]uint16
	ApplicationType     uint32
	AppStatus           uint32
	TSSessionId         uint32
	BRestartable        int32
}

// LockingProcess describes an application holding an open file handle on a target path.
type LockingProcess struct {
	PID         uint32 `json:"pid"`
	AppName     string `json:"appName"`
	ServiceName string `json:"serviceName"`
	ExePath     string `json:"exePath"`
	Protected   bool   `json:"protected"`
}

// FindLockingProcesses uses the Windows Restart Manager API (rstrtmgr.dll) to identify
// which running processes are currently locking a file or directory.
func FindLockingProcesses(targetPath string) ([]LockingProcess, error) {
	if targetPath == "" {
		return nil, fmt.Errorf("target path is empty")
	}
	fi, err := os.Stat(targetPath)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}

	var filePaths []string
	if fi.IsDir() {
		// RmRegisterResources operates on file paths; collect up to 150 files inside directory
		_ = filepath.Walk(targetPath, func(p string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return nil
			}
			filePaths = append(filePaths, p)
			if len(filePaths) >= 150 {
				return filepath.SkipDir
			}
			return nil
		})
		if len(filePaths) == 0 {
			return []LockingProcess{}, nil
		}
	} else {
		filePaths = []string{targetPath}
	}

	if err := procRmStartSession.Find(); err != nil {
		return nil, fmt.Errorf("Restart Manager API unavailable: %w", err)
	}

	var sessionHandle uint32
	var sessionKey [cchRmSessionKey + 1]uint16
	ret, _, _ := procRmStartSession.Call(
		uintptr(unsafe.Pointer(&sessionHandle)),
		0,
		uintptr(unsafe.Pointer(&sessionKey[0])),
	)
	if ret != 0 {
		return nil, fmt.Errorf("RmStartSession failed with code %d", ret)
	}
	defer procRmEndSession.Call(uintptr(sessionHandle))

	// Convert file paths to UTF-16 pointer array
	ptrs := make([]*uint16, 0, len(filePaths))
	for _, fp := range filePaths {
		if u, err := windows.UTF16PtrFromString(fp); err == nil {
			ptrs = append(ptrs, u)
		}
	}
	if len(ptrs) == 0 {
		return []LockingProcess{}, nil
	}

	ret, _, _ = procRmRegisterResources.Call(
		uintptr(sessionHandle),
		uintptr(len(ptrs)),
		uintptr(unsafe.Pointer(&ptrs[0])),
		0, 0, 0, 0,
	)
	if ret != 0 {
		return nil, fmt.Errorf("RmRegisterResources failed with code %d", ret)
	}

	var procInfoNeeded uint32
	var procInfo uint32
	var rebootReasons uint32

	// First call to get required buffer size
	ret, _, _ = procRmGetList.Call(
		uintptr(sessionHandle),
		uintptr(unsafe.Pointer(&procInfoNeeded)),
		uintptr(unsafe.Pointer(&procInfo)),
		0,
		uintptr(unsafe.Pointer(&rebootReasons)),
	)

	// ERROR_MORE_DATA = 234
	if ret == 0 && procInfoNeeded == 0 {
		return []LockingProcess{}, nil
	}
	if ret != 234 && ret != 0 {
		return nil, fmt.Errorf("RmGetList failed with code %d", ret)
	}

	if procInfoNeeded == 0 {
		return []LockingProcess{}, nil
	}

	affected := make([]rmProcessInfo, procInfoNeeded)
	procInfo = procInfoNeeded
	ret, _, _ = procRmGetList.Call(
		uintptr(sessionHandle),
		uintptr(unsafe.Pointer(&procInfoNeeded)),
		uintptr(unsafe.Pointer(&procInfo)),
		uintptr(unsafe.Pointer(&affected[0])),
		uintptr(unsafe.Pointer(&rebootReasons)),
	)
	if ret != 0 && ret != 234 {
		return nil, fmt.Errorf("RmGetList retrieval failed with code %d", ret)
	}

	seen := make(map[uint32]bool)
	selfPID := uint32(os.Getpid())
	var results []LockingProcess

	for i := uint32(0); i < procInfo && int(i) < len(affected); i++ {
		info := affected[i]
		pid := info.Process.dwProcessId
		if pid == 0 || pid == selfPID || seen[pid] {
			continue
		}
		seen[pid] = true

		appName := windows.UTF16ToString(info.StrAppName[:])
		svcName := windows.UTF16ToString(info.StrServiceShortName[:])
		exe := imagePath(pid)

		results = append(results, LockingProcess{
			PID:         pid,
			AppName:     appName,
			ServiceName: svcName,
			ExePath:     exe,
			Protected:   isProtectedProcess(filepath.Base(exe), pid),
		})
	}

	return results, nil
}
