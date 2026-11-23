package backend

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

type PowerBlocker struct {
	Category string `json:"category"` // SYSTEM, DISPLAY, AWAYMODE, EXECUTION, PERFBOOST
	Caller   string `json:"caller"`   // e.g. "[PROCESS] \Device\HarddiskVolume3\...\Spotify.exe"
	Reason   string `json:"reason"`   // e.g. "An audio stream is currently in use."
	ExePath  string `json:"exePath"`  // resolved exe path if it was a process
}

type PowerReport struct {
	Available     bool           `json:"available"`
	RequiresAdmin bool           `json:"requiresAdmin"`
	HasBlockers   bool           `json:"hasBlockers"`
	Blockers      []PowerBlocker `json:"blockers"`
	Summary       string         `json:"summary"`
	Error         string         `json:"error,omitempty"`
}

// GetPowerReport checks active Windows Power Requests (sleep and display idle blockers).
// Requires administrator elevation to query powercfg requests.
func GetPowerReport() PowerReport {
	elevated := isElevated()
	if !elevated {
		return PowerReport{
			Available:     false,
			RequiresAdmin: true,
			Summary:       "Administrator rights are required to query active sleep and display blockers.",
		}
	}

	cmd := exec.Command("powercfg", "/requests")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	if err != nil {
		return PowerReport{
			Available: false,
			Error:     "Failed to execute powercfg: " + err.Error(),
			Summary:   "Unable to inspect power requests.",
		}
	}

	rawText := string(out)
	report := PowerReport{
		Available: true,
	}

	scanner := bufio.NewScanner(strings.NewReader(rawText))
	currentCategory := ""
	var pendingCaller string

	categories := map[string]bool{
		"DISPLAY:":          true,
		"SYSTEM:":           true,
		"AWAYMODE:":         true,
		"EXECUTION:":        true,
		"PERFBOOST:":        true,
		"ACTIVELOCKSCREEN:": true,
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		upperLine := strings.ToUpper(line)
		if categories[upperLine] {
			currentCategory = strings.TrimSuffix(upperLine, ":")
			pendingCaller = ""
			continue
		}

		if strings.EqualFold(line, "None.") {
			pendingCaller = ""
			continue
		}

		if pendingCaller == "" {
			// First line of an entry is the caller (e.g. "[PROCESS] ...")
			pendingCaller = line
		} else {
			// Second line of an entry is the reason
			reason := line
			exe := ""
			if strings.HasPrefix(strings.ToUpper(pendingCaller), "[PROCESS]") {
				rawExe := strings.TrimSpace(strings.TrimPrefix(pendingCaller, "[PROCESS]"))
				rawExe = strings.TrimPrefix(rawExe, "[process]")
				rawExe = strings.TrimSpace(rawExe)
				exe = exePath(rawExe)
			}

			report.Blockers = append(report.Blockers, PowerBlocker{
				Category: currentCategory,
				Caller:   pendingCaller,
				Reason:   reason,
				ExePath:  exe,
			})
			report.HasBlockers = true
			pendingCaller = ""
		}
	}

	if report.HasBlockers {
		var names []string
		for _, b := range report.Blockers {
			name := b.Caller
			if b.ExePath != "" {
				name = b.ExePath
			}
			names = append(names, fmt.Sprintf("%s (%s)", name, b.Category))
		}
		report.Summary = fmt.Sprintf("%d active blocker(s) preventing sleep or display turn-off: %s.", len(report.Blockers), strings.Join(names, ", "))
	} else {
		report.Summary = "No apps or drivers are currently blocking Windows sleep."
	}

	return report
}
