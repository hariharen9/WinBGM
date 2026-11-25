package backend

import (
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modSrClient           = windows.NewLazySystemDLL("srclient.dll")
	procSRSetRestorePoint = modSrClient.NewProc("SRSetRestorePointW")
)

const (
	beginSystemChange = 100
	endSystemChange   = 101
	modifySettings    = 12
	maxDescW          = 256
)

type restorePointInfoW struct {
	dwEventType      uint32
	dwRestorePtType  uint32
	llSequenceNumber int64
	szDescription    [maxDescW]uint16
}

type stateMgrStatus struct {
	nStatus          uint32
	llSequenceNumber int64
}

// RestorePointResult describes the outcome of creating a Windows System Restore Point.
type RestorePointResult struct {
	Success        bool   `json:"success"`
	SequenceNumber int64  `json:"sequenceNumber"`
	Description    string `json:"description"`
	Message        string `json:"message"`
	CreatedAt      string `json:"createdAt"`
}

// CreateSystemRestorePoint creates a native Windows System Restore Point via srclient.dll.
// Requires Administrator privileges and System Protection enabled on the OS drive.
func CreateSystemRestorePoint(description string) (RestorePointResult, error) {
	if !isElevated() {
		return RestorePointResult{}, fmt.Errorf("administrator rights required to create a System Restore Point")
	}
	if err := procSRSetRestorePoint.Find(); err != nil {
		return RestorePointResult{}, fmt.Errorf("Windows System Restore API (srclient.dll) is unavailable: %w", err)
	}

	desc := strings.TrimSpace(description)
	if desc == "" {
		desc = "WinBGM Safety Checkpoint"
	}

	var info restorePointInfoW
	info.dwEventType = beginSystemChange
	info.dwRestorePtType = modifySettings
	info.llSequenceNumber = 0

	utf16Desc, err := windows.UTF16FromString(desc)
	if err != nil {
		return RestorePointResult{}, err
	}
	copy(info.szDescription[:maxDescW-1], utf16Desc)

	var status stateMgrStatus
	ret, _, callErr := procSRSetRestorePoint.Call(
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Pointer(&status)),
	)
	if ret == 0 || status.nStatus != 0 {
		return RestorePointResult{}, formatRestoreError(status.nStatus, callErr)
	}

	// Commit the restore point
	var endInfo restorePointInfoW
	endInfo.dwEventType = endSystemChange
	endInfo.dwRestorePtType = modifySettings
	endInfo.llSequenceNumber = status.llSequenceNumber

	var endStatus stateMgrStatus
	retEnd, _, callErrEnd := procSRSetRestorePoint.Call(
		uintptr(unsafe.Pointer(&endInfo)),
		uintptr(unsafe.Pointer(&endStatus)),
	)
	if retEnd == 0 || endStatus.nStatus != 0 {
		return RestorePointResult{}, formatRestoreError(endStatus.nStatus, callErrEnd)
	}

	return RestorePointResult{
		Success:        true,
		SequenceNumber: status.llSequenceNumber,
		Description:    desc,
		Message:        fmt.Sprintf("System Restore Point \"%s\" created (Sequence #%d).", desc, status.llSequenceNumber),
		CreatedAt:      time.Now().Format(time.RFC3339),
	}, nil
}

func formatRestoreError(nStatus uint32, callErr error) error {
	switch nStatus {
	case 1058: // ERROR_SERVICE_DISABLED
		return fmt.Errorf("System Restore is disabled in Windows. Enable System Protection for C: in 'Create a restore point' settings first")
	case 1314: // ERROR_PRIVILEGE_NOT_HELD
		return fmt.Errorf("administrator privileges required to create a System Restore Point")
	case 112: // ERROR_DISK_FULL
		return fmt.Errorf("insufficient disk space to create a System Restore Point")
	case 0:
		if callErr != nil && callErr != windows.ERROR_SUCCESS {
			return fmt.Errorf("failed to create System Restore Point: %v", callErr)
		}
		return fmt.Errorf("failed to create System Restore Point")
	default:
		return fmt.Errorf("Windows System Restore returned status code %d", nStatus)
	}
}
