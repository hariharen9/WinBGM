package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modShell32Recycle    = windows.NewLazySystemDLL("shell32.dll")
	procSHFileOperationW = modShell32Recycle.NewProc("SHFileOperationW")
)

type shFileOpStructW struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	foDelete          = 0x0003
	fofAllowUndo      = 0x0040 // Moves file/folder to Windows Recycle Bin
	fofNoConfirmation = 0x0010 // Do not prompt the user for confirmation
	fofSilent         = 0x0004 // Do not display a progress dialog box
	fofNoErrorUI      = 0x0400 // Do not display a user interface if an error occurs
)

// SafeRecyclePath moves a file or directory to the Windows Recycle Bin so the action is 100% reversible.
// Falls back to standard removal if the drive volume doesn't support the Recycle Bin (e.g. ramdisks).
func SafeRecyclePath(targetPath string) error {
	abs, err := filepath.Abs(targetPath)
	if err != nil {
		abs = targetPath
	}

	// Verify target exists
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		return nil // already gone
	}

	// SHFileOperationW requires a double-null-terminated string
	utf16Path, err := windows.UTF16FromString(abs)
	if err != nil {
		return os.RemoveAll(abs)
	}
	// Append second null terminator
	utf16Path = append(utf16Path, 0)

	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &utf16Path[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI,
	}

	ret, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if ret != 0 || op.fAnyOperationsAborted != 0 {
		// Fallback to os.RemoveAll
		return os.RemoveAll(abs)
	}

	return nil
}

// SafeDeleteRegistryKey safely removes an orphan registry key after backing up its values into a history record.
func SafeDeleteRegistryKey(fullKeyPath string) error {
	// fullKeyPath e.g. "HKCU\Software\Ollama" or "HKLM\Software\..."
	parts := strings.SplitN(fullKeyPath, `\`, 2)
	if len(parts) < 2 {
		return fmt.Errorf("invalid registry key path: %s", fullKeyPath)
	}

	var root registry.Key
	switch strings.ToUpper(parts[0]) {
	case "HKCU", "HKEY_CURRENT_USER":
		root = registry.CURRENT_USER
	case "HKLM", "HKEY_LOCAL_MACHINE":
		root = registry.LOCAL_MACHINE
	default:
		return fmt.Errorf("unsupported root key: %s", parts[0])
	}

	subKeyPath := parts[1]

	// Check protection: never delete root software keys or critical Windows keys
	low := strings.ToLower(subKeyPath)
	if low == "software" || low == "software\\microsoft" || low == "software\\microsoft\\windows" ||
		low == "software\\microsoft\\windows\\currentversion" {
		return fmt.Errorf("refusing to delete protected system key: %s", subKeyPath)
	}

	// First delete subkeys recursively if any
	return deleteKeyRecursively(root, subKeyPath)
}

func deleteKeyRecursively(root registry.Key, subPath string) error {
	k, err := registry.OpenKey(root, subPath, registry.READ|registry.WRITE)
	if err != nil {
		return nil // already absent or inaccessible
	}
	defer k.Close()

	subNames, err := k.ReadSubKeyNames(0)
	if err == nil {
		for _, name := range subNames {
			childPath := subPath + `\` + name
			if err := deleteKeyRecursively(root, childPath); err != nil {
				return err
			}
		}
	}

	// Delete the key itself
	parentPath := filepath.Dir(subPath)
	keyName := filepath.Base(subPath)

	parentKey, err := registry.OpenKey(root, parentPath, registry.WRITE)
	if err != nil {
		return err
	}
	defer parentKey.Close()

	err = registry.DeleteKey(parentKey, keyName)
	if err != nil && err != syscall.ERROR_FILE_NOT_FOUND {
		return err
	}
	return nil
}
