package backend

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
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
		// Portable / Orphaned apps have no vendor uninstaller; return 0 so caller proceeds to residual Recycle Bin cleanup
		if strings.EqualFold(app.Source, "Orphaned") {
			return 0, nil
		}
		return -1, errors.New("no uninstall command string provided for this application")
	}

	// Handle Steam protocol URLs (steam://uninstall/<appid>)
	if strings.HasPrefix(strings.ToLower(cmdStr), "steam://") {
		c := exec.Command("cmd.exe", "/c", "start", "", cmdStr)
		c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := c.Run(); err != nil {
			return -1, err
		}
		return 0, nil
	}

	// Handle Scoop / Chocolatey / Winget / StoreApp PowerShell uninstalls
	lowCmd := strings.ToLower(cmdStr)
	if strings.HasPrefix(lowCmd, "scoop ") || strings.HasPrefix(lowCmd, "choco ") ||
		strings.HasPrefix(lowCmd, "winget ") || strings.Contains(lowCmd, "remove-appxpackage") {
		psCmd := cmdStr
		if strings.Contains(lowCmd, "remove-appxpackage") && strings.Contains(lowCmd, "-command ") {
			// Already wrapped in powershell.exe -NoProfile -Command "..."
			idx := strings.Index(lowCmd, "-command ")
			psCmd = strings.Trim(strings.TrimSpace(cmdStr[idx+len("-command "):]), `"`)
		}
		c := exec.Command("powershell.exe", "-NoProfile", "-Command", psCmd)
		c.SysProcAttr = &syscall.SysProcAttr{HideWindow: strings.Contains(lowCmd, "remove-appxpackage")}
		if err := c.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode(), nil
			}
			return -1, err
		}
		return 0, nil
	}

	// Resolve executable path without corrupting quoted args or unquoted paths with spaces,
	// and pass the raw command line via SysProcAttr.CmdLine.
	exe := exePath(cmdStr)
	if exe == "" {
		parts := strings.Fields(cmdStr)
		if len(parts) > 0 {
			exe = strings.Trim(parts[0], `"`)
		}
	}
	if exe == "" {
		return -1, fmt.Errorf("failed to parse uninstaller executable from: %s", cmdStr)
	}

	finalCmdLine := cmdStr
	if strings.EqualFold(filepathBase(exe), "msiexec.exe") || strings.EqualFold(filepathBase(exe), "msiexec") {
		if quiet {
			low := strings.ToLower(finalCmdLine)
			if !strings.Contains(low, " /qn") && !strings.Contains(low, " /quiet") && !strings.Contains(low, " /q") {
				finalCmdLine += " /qn"
			}
		}
	}

	cmd := exec.Command(exe)
	// Allow child GUI uninstaller windows to open normally and preserve exact quoting/args
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: false,
		CmdLine:    finalCmdLine,
	}

	code, err := runAndWaitProcessTree(cmd)
	// Allow brief OS filesystem/registry flush before residual scan runs
	time.Sleep(600 * time.Millisecond)
	return code, err
}

// runAndWaitProcessTree starts cmd inside a Windows Job Object and waits for both
// the main process and any spawned child processes (e.g. Inno Setup / NSIS temp
// uninstallers in %TEMP%) to exit before returning.
func runAndWaitProcessTree(cmd *exec.Cmd) (int, error) {
	job, jobErr := windows.CreateJobObject(nil, nil)
	if jobErr == nil {
		defer windows.CloseHandle(job)
	}

	if err := cmd.Start(); err != nil {
		return -1, err
	}

	assigned := false
	if jobErr == nil && cmd.Process != nil {
		if procH, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid)); err == nil {
			if err := windows.AssignProcessToJobObject(job, procH); err == nil {
				assigned = true
			}
			windows.CloseHandle(procH)
		}
	}

	exitCode := 0
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return -1, err
		}
	}

	if assigned {
		waitForJobTreeExit(job, 5*time.Minute)
	}

	return exitCode, nil
}

func waitForJobTreeExit(job windows.Handle, maxWait time.Duration) {
	type jobBasicProcessIDList struct {
		NumberOfAssignedProcesses uint32
		NumberOfProcessIdsInList  uint32
		ProcessIdList             [64]uintptr
	}

	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		var list jobBasicProcessIDList
		var retLen uint32
		err := windows.QueryInformationJobObject(
			job,
			windows.JobObjectBasicProcessIdList,
			uintptr(unsafe.Pointer(&list)),
			uint32(unsafe.Sizeof(list)),
			&retLen,
		)
		if err != nil || list.NumberOfAssignedProcesses == 0 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func filepathBase(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx >= 0 {
		return path[idx+1:]
	}
	return path
}
