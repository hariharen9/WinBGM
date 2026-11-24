package backend

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// LaunchUninstaller executes the vendor uninstaller command for an app.
// If quiet is true and QuietUninstallString exists, it runs silent mode.
// It waits for the uninstaller process to complete so that residual scanning
// can immediately analyze what was left behind.
func LaunchUninstaller(app InstalledApp, quiet bool) (int, error) {
	cmdStr := app.UninstallString
	if quiet && app.QuietUninstall != "" {
		cmdStr = app.QuietUninstall
	}

	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return -1, errors.New("no uninstall command string provided for this application")
	}

	// Parse command string into executable and arguments
	var exe string
	var args []string

	if strings.HasPrefix(cmdStr, `"`) {
		endIdx := strings.Index(cmdStr[1:], `"`)
		if endIdx > 0 {
			exe = cmdStr[1 : endIdx+1]
			remainder := strings.TrimSpace(cmdStr[endIdx+2:])
			if remainder != "" {
				args = strings.Fields(remainder)
			}
		} else {
			exe = strings.Trim(cmdStr, `"`)
		}
	} else {
		parts := strings.Fields(cmdStr)
		if len(parts) > 0 {
			exe = parts[0]
			args = parts[1:]
		}
	}

	if exe == "" {
		return -1, fmt.Errorf("failed to parse uninstaller executable from: %s", cmdStr)
	}

	// Special handling for MsiExec
	if strings.EqualFold(filepathBase(exe), "msiexec.exe") || strings.EqualFold(filepathBase(exe), "msiexec") {
		if quiet {
			// Add quiet flag if not already present
			hasQuiet := false
			for _, a := range args {
				if strings.EqualFold(a, "/qn") || strings.EqualFold(a, "/quiet") || strings.EqualFold(a, "/q") {
					hasQuiet = true
					break
				}
			}
			if !hasQuiet {
				args = append(args, "/qn")
			}
		}
	}

	cmd := exec.Command(exe, args...)
	// Allow child GUI uninstaller windows to open normally
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: false,
	}

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}

	return 0, nil
}

func filepathBase(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx >= 0 {
		return path[idx+1:]
	}
	return path
}
