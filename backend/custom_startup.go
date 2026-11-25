package backend

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// CustomStartupSpec defines a user-created startup command or delayed-launch task.
type CustomStartupSpec struct {
	Name         string `json:"name"`
	Command      string `json:"command"`
	Args         string `json:"args"`
	WorkingDir   string `json:"workingDir"`
	DelaySeconds int    `json:"delaySeconds"`
	RunHidden    bool   `json:"runHidden"`
	Elevated     bool   `json:"elevated"`
}

func sanitizeTaskName(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ' ':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	res := strings.TrimSpace(b.String())
	if res == "" {
		return "CustomEntry"
	}
	return res
}

func escapeXMLText(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

// CreateCustomStartup creates either a direct HKCU Run entry (0s delay, standard)
// or a native Windows Task Scheduler logon task with <Delay>PT{N}S</Delay>.
func CreateCustomStartup(spec CustomStartupSpec) error {
	name := strings.TrimSpace(spec.Name)
	cmdPath := strings.TrimSpace(spec.Command)
	args := strings.TrimSpace(spec.Args)
	workDir := strings.TrimSpace(spec.WorkingDir)

	if name == "" {
		return errors.New("entry name is required")
	}
	if cmdPath == "" {
		return errors.New("command or executable path is required")
	}
	if spec.DelaySeconds < 0 {
		spec.DelaySeconds = 0
	}

	// Simple instant startup -> HKCU\...\Run
	if spec.DelaySeconds == 0 && !spec.Elevated && !spec.RunHidden && workDir == "" {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("open HKCU Run key: %w", err)
		}
		defer k.Close()

		val := cmdPath
		if strings.Contains(cmdPath, " ") && !strings.HasPrefix(cmdPath, `"`) {
			val = `"` + cmdPath + `"`
		}
		if args != "" {
			val += " " + args
		}
		if err := k.SetStringValue(name, val); err != nil {
			return fmt.Errorf("write Run value: %w", err)
		}
		return nil
	}

	// Delayed / Hidden / Elevated / Custom WorkingDir -> Task Scheduler LogonTrigger XML
	if workDir == "" && filepath.IsAbs(cmdPath) {
		workDir = filepath.Dir(cmdPath)
	}
	runLevel := "LeastPrivilege"
	if spec.Elevated {
		runLevel = "HighestAvailable"
	}
	delayXML := ""
	if spec.DelaySeconds > 0 {
		delayXML = fmt.Sprintf("\n      <Delay>PT%dS</Delay>", spec.DelaySeconds)
	}
	argsXML := ""
	if args != "" {
		argsXML = fmt.Sprintf("\n      <Arguments>%s</Arguments>", escapeXMLText(args))
	}
	workDirXML := ""
	if workDir != "" {
		workDirXML = fmt.Sprintf("\n      <WorkingDirectory>%s</WorkingDirectory>", escapeXMLText(workDir))
	}

	taskXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Author>WinBGM</Author>
    <Description>Custom startup entry created by WinBGM</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>%s
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>%s</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>%t</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>%s%s
    </Exec>
  </Actions>
</Task>`, delayXML, runLevel, spec.RunHidden, escapeXMLText(cmdPath), argsXML, workDirXML)

	tmp, err := os.CreateTemp("", "winbgm-task-*.xml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	// Encode as UTF-16LE with BOM for schtasks /XML compatibility.
	var utf16Buf bytes.Buffer
	utf16Buf.Write([]byte{0xFF, 0xFE})
	for _, r := range taskXML {
		utf16Buf.WriteByte(byte(r))
		utf16Buf.WriteByte(byte(r >> 8))
	}
	if _, err := tmp.Write(utf16Buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	taskName := `\WinBGM\Custom\` + sanitizeTaskName(name)
	cmd := exec.Command("schtasks.exe", "/Create", "/TN", taskName, "/XML", tmpPath, "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks create failed: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}
