package backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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

var protectedFolderNames = map[string]bool{
	"microsoft": true, "windows": true, "google": true, "adobe": true,
	"intel": true, "amd": true, "nvidia": true, "apple": true,
	"common files": true, "windowsapps": true, "system32": true,
	"syswow64": true, "program files": true, "program files (x86)": true,
	"users": true, "programdata": true, "appdata": true, "local": true,
	"roaming": true, "temp": true, "desktop": true, "documents": true,
	"downloads": true, "pictures": true, "videos": true, "music": true,
}

// ValidateCleanPath ensures a file or directory path is safe to move to the Recycle Bin.
// Blocks drive roots, Windows OS directories, user profile roots, and shared vendor roots.
func ValidateCleanPath(targetPath string) error {
	if strings.TrimSpace(targetPath) == "" {
		return fmt.Errorf("empty path")
	}
	abs, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", targetPath, err)
	}
	clean := filepath.Clean(abs)
	if len(clean) <= 3 {
		return fmt.Errorf("refusing to clean drive root: %s", clean)
	}

	low := strings.ToLower(clean)

	// Block Windows OS directory and everything inside it
	sysRoot := strings.ToLower(filepath.Clean(os.Getenv("SystemRoot")))
	if sysRoot != "" && (low == sysRoot || strings.HasPrefix(low, sysRoot+`\`)) {
		return fmt.Errorf("refusing to clean Windows system path: %s", clean)
	}

	// Block exact special roots
	envRoots := []string{
		os.Getenv("USERPROFILE"),
		os.Getenv("APPDATA"),
		os.Getenv("LOCALAPPDATA"),
		os.Getenv("ProgramData"),
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
	}
	for _, r := range envRoots {
		if r == "" {
			continue
		}
		cr := strings.ToLower(filepath.Clean(r))
		if low == cr {
			return fmt.Errorf("refusing to clean root folder: %s", clean)
		}
		// Never clean a path that is an ancestor of a special root
		if strings.HasPrefix(cr, low+`\`) {
			return fmt.Errorf("refusing to clean parent of system/user root: %s", clean)
		}
	}

	// Block shared vendor/system folder names directly under AppData, ProgramData, or Program Files
	baseName := strings.ToLower(filepath.Base(clean))
	if protectedFolderNames[baseName] {
		parent := strings.ToLower(filepath.Dir(clean))
		for _, r := range envRoots {
			if r != "" && parent == strings.ToLower(filepath.Clean(r)) {
				return fmt.Errorf("refusing to clean shared vendor/system directory: %s", clean)
			}
		}
	}

	return nil
}

// SafeRecyclePath moves a file or directory to the Windows Recycle Bin.
// It NEVER permanently deletes: if the Recycle Bin operation fails or is aborted, it returns an error.
func SafeRecyclePath(targetPath string) error {
	if err := ValidateCleanPath(targetPath); err != nil {
		return err
	}
	abs, err := filepath.Abs(targetPath)
	if err != nil {
		return err
	}

	if _, err := os.Stat(abs); os.IsNotExist(err) {
		return nil // already gone
	}

	// SHFileOperationW requires a double-null-terminated string
	utf16Path, err := windows.UTF16FromString(abs)
	if err != nil {
		return err
	}
	utf16Path = append(utf16Path, 0)

	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &utf16Path[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI,
	}

	ret, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if ret != 0 || op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("Recycle Bin move failed for %q (code=%d, aborted=%d); permanent deletion refused", abs, ret, op.fAnyOperationsAborted)
	}

	return nil
}

// ValidateCleanRegistryPath ensures a registry key path is specific and not a protected OS/vendor root.
func ValidateCleanRegistryPath(fullKeyPath string) (registry.Key, string, error) {
	parts := strings.SplitN(fullKeyPath, `\`, 2)
	if len(parts) < 2 {
		return 0, "", fmt.Errorf("invalid registry key path: %s", fullKeyPath)
	}

	var root registry.Key
	switch strings.ToUpper(parts[0]) {
	case "HKCU", "HKEY_CURRENT_USER":
		root = registry.CURRENT_USER
	case "HKLM", "HKEY_LOCAL_MACHINE":
		root = registry.LOCAL_MACHINE
	default:
		return 0, "", fmt.Errorf("unsupported root key: %s", parts[0])
	}

	subKeyPath := strings.Trim(parts[1], `\`)
	low := strings.ToLower(subKeyPath)
	if !strings.HasPrefix(low, `software\`) {
		return 0, "", fmt.Errorf("refusing to modify non-Software registry path: %s", fullKeyPath)
	}
	rest := strings.TrimPrefix(low, `software\`)
	if rest == "" || protectedFolderNames[rest] || strings.HasPrefix(rest, `microsoft\`) || strings.HasPrefix(rest, `policies\`) {
		return 0, "", fmt.Errorf("refusing to delete protected system or vendor registry key: %s", fullKeyPath)
	}

	return root, subKeyPath, nil
}

func exportRegistryBackup(fullKeyPath string) (string, error) {
	backupDir := filepath.Join(os.Getenv("APPDATA"), "WinBGM", "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create registry backup directory: %w", err)
	}

	safeName := strings.NewReplacer(`\`, "_", `/`, "_", `:`, "_", ` `, "_").Replace(fullKeyPath)
	if len(safeName) > 64 {
		safeName = safeName[len(safeName)-64:]
	}
	backupFile := filepath.Join(backupDir, fmt.Sprintf("%s_%s.reg", time.Now().Format("20060102_150405"), safeName))

	// Normalize root prefix for reg.exe
	regPath := fullKeyPath
	if strings.HasPrefix(strings.ToUpper(regPath), `HKCU\`) {
		regPath = `HKEY_CURRENT_USER\` + regPath[5:]
	} else if strings.HasPrefix(strings.ToUpper(regPath), `HKLM\`) {
		regPath = `HKEY_LOCAL_MACHINE\` + regPath[5:]
	}

	cmd := exec.Command("reg.exe", "export", regPath, backupFile, "/y")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("reg export backup failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return backupFile, nil
}

// SafeDeleteRegistryKey exports a .reg backup first, logs to history, and only then removes the key.
func SafeDeleteRegistryKey(fullKeyPath string) error {
	root, subKeyPath, err := ValidateCleanRegistryPath(fullKeyPath)
	if err != nil {
		return err
	}

	// Verify key exists before backing up
	k, err := registry.OpenKey(root, subKeyPath, registry.READ)
	if err != nil {
		return nil // already absent
	}
	k.Close()

	backupFile, err := exportRegistryBackup(fullKeyPath)
	if err != nil {
		return fmt.Errorf("aborting registry deletion because backup failed: %w", err)
	}

	if err := deleteKeyRecursively(root, subKeyPath); err != nil {
		return err
	}

	_ = appendHistory(HistoryEntry{
		ID:        fmt.Sprintf("regclean-%d", time.Now().UnixNano()),
		Time:      time.Now().Format(time.RFC3339),
		Title:     "Cleaned registry key: " + filepath.Base(subKeyPath),
		Summary:   fmt.Sprintf("Exported .reg backup to %s before removal", backupFile),
		Status:    "applied",
		Restorable: false,
	})
	return nil
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
