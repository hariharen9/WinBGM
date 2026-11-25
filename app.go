package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"winbgm/backend"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is bound to the frontend.
type App struct {
	ctx            context.Context
	last           *backend.Inventory
	installedCache []backend.InstalledApp
	minimizeToTray bool
	forceQuit      bool
}

func NewApp() *App { return &App{} }

func trayFlagPath() string {
	return filepath.Join(os.Getenv("APPDATA"), "WinBGM", "tray.flag")
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if _, err := os.Stat(trayFlagPath()); err == nil {
		a.minimizeToTray = true
	}

	// Lightweight real-time process, RAM & network socket polling every 3 seconds
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				res := backend.PollProcesses()
				runtime.EventsEmit(a.ctx, "process_poll", res)
			}
		}
	}()

	// Start Sentinel background persistence watcher with native Windows Toast alerts
	backend.StartSentinel(a.ctx, func(alert backend.SentinelAlert) {
		runtime.EventsEmit(a.ctx, "sentinel_alert", alert)
		backend.SendWindowsToast(
			"WinBGM Watchdog Alert",
			fmt.Sprintf("%s registered a startup entry (%s). Open WinBGM to Keep or Disable Safely.", alert.Name, alert.RootKey),
		)
	})

	// Start System Tray daemon
	backend.StartTray(
		func() {
			runtime.WindowShow(a.ctx)
			runtime.WindowUnminimise(a.ctx)
		},
		func() {
			runtime.WindowShow(a.ctx)
			runtime.EventsEmit(a.ctx, "trigger_scan", nil)
		},
		func() {
			a.forceQuit = true
			runtime.Quit(a.ctx)
		},
	)
}

func (a *App) onBeforeClose(ctx context.Context) bool {
	if a.minimizeToTray && !a.forceQuit {
		runtime.WindowHide(ctx)
		return true // Prevent app exit; stay alive in system tray
	}
	return false
}

func (a *App) shutdown(ctx context.Context) {
	backend.StopTray()
}

// SuspendProcess freezes all threads of a running process without terminating it.
func (a *App) SuspendProcess(pid uint32) error {
	return backend.SuspendProcess(pid)
}

// ResumeProcess unfreezes a suspended process.
func (a *App) ResumeProcess(pid uint32) error {
	return backend.ResumeProcess(pid)
}

// GracefulCloseProcess sends WM_CLOSE to process windows.
func (a *App) GracefulCloseProcess(pid uint32) error {
	return backend.GracefulCloseProcess(pid)
}

// KillProcess forcefully terminates a process.
func (a *App) KillProcess(pid uint32) error {
	return backend.KillProcess(pid)
}

// IsProcessSuspended checks if a process was suspended by WinBGM.
func (a *App) IsProcessSuspended(pid uint32) bool {
	return backend.IsProcessSuspended(pid)
}

// SaveSnapshot saves a baseline snapshot to disk.
func (a *App) SaveSnapshot(name string) (backend.SnapshotMeta, error) {
	if a.last == nil {
		inv := backend.BuildInventory()
		a.last = &inv
	}
	return backend.SaveSnapshot(name, *a.last)
}

// ListSnapshots returns all saved baseline snapshots.
func (a *App) ListSnapshots() ([]backend.SnapshotMeta, error) {
	return backend.ListSnapshots()
}

// DeleteSnapshot removes a snapshot.
func (a *App) DeleteSnapshot(id string) error {
	return backend.DeleteSnapshot(id)
}

// DiffSnapshots compares a saved baseline snapshot against the live inventory.
func (a *App) DiffSnapshots(baseID string) (backend.SnapshotDiff, error) {
	if a.last == nil {
		inv := backend.BuildInventory()
		a.last = &inv
	}
	return backend.DiffSnapshots(baseID, a.last)
}

// GetSentinelAlerts returns active re-add sentinel notifications.
func (a *App) GetSentinelAlerts() []backend.SentinelAlert {
	return backend.GetSentinelAlerts()
}

// ClearSentinelAlerts clears active re-add alerts.
func (a *App) ClearSentinelAlerts() {
	backend.ClearSentinelAlerts()
}

// PollProcesses runs an immediate lightweight process & RAM snapshot (<5ms).
func (a *App) PollProcesses() backend.ProcessPollResult {
	return backend.PollProcesses()
}

// RestoreBatch undoes all restorable changes from a batch session in a single action.
func (a *App) RestoreBatch(batchID string) backend.BatchResult {
	return backend.RestoreBatch(batchID)
}

// GetBootProfile returns boot duration metrics and app boot degradation delays from Windows Diagnostics.
func (a *App) GetBootProfile() backend.BootProfile {
	return backend.GetBootProfile()
}

// GetPowerReport checks active sleep and display turn-off blocker requests.
func (a *App) GetPowerReport() backend.PowerReport {
	return backend.GetPowerReport()
}

// Scan inspects the machine and returns the full inventory. It changes nothing.
func (a *App) Scan() backend.Inventory {
	inv := backend.BuildInventory()
	a.last = &inv
	return inv
}

// Apply changes one item after re-checking every safety rule on the Go side.
func (a *App) Apply(req backend.ChangeRequest) backend.ChangeResult {
	return backend.Apply(a.last, req)
}

// Restore undoes one history entry, but only if the item is still exactly as WinBGM left it.
func (a *App) Restore(id string) backend.ChangeResult {
	return backend.Restore(id)
}

// History returns past changes, newest first.
func (a *App) History() []backend.HistoryEntry {
	return backend.History()
}

// ApplyFocusMode disables third-party automatic startup items safely.
func (a *App) ApplyFocusMode() backend.BatchResult {
	return backend.ApplyFocusMode(a.last)
}

// GetAlwaysAdmin returns true if the app executable has the RUNASADMIN compatibility flag set.
func (a *App) GetAlwaysAdmin() bool {
	return backend.CheckAlwaysAdmin()
}

// SetAlwaysAdmin enables or disables the RUNASADMIN compatibility flag in Windows registry (HKCU).
func (a *App) SetAlwaysAdmin(enable bool) (bool, error) {
	return backend.SetAlwaysAdmin(enable)
}

// RestartAsAdmin relaunches this exe with a UAC prompt, then closes this window.
func (a *App) RestartAsAdmin() error {
	if err := backend.RelaunchAsAdmin(); err != nil {
		return err
	}
	runtime.Quit(a.ctx)
	return nil
}

// Reveal opens File Explorer with the given file selected (or its folder if the file is gone).
func (a *App) Reveal(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("no location available")
	}
	target := path
	if _, err := os.Stat(path); err != nil {
		target = filepath.Dir(path)
		if _, err := os.Stat(target); err != nil {
			return errors.New("location not found")
		}
		c := exec.Command("explorer.exe", target)
		return c.Start()
	}
	c := exec.Command("explorer.exe")
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + target + `"`}
	return c.Start()
}

// Export writes the last scan to a JSON file chosen by the user. Returns the path ("" if cancelled).
func (a *App) Export() (string, error) {
	if a.last == nil {
		return "", errors.New("run a scan first")
	}
	p, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export background inventory (JSON)",
		DefaultFilename: "background-inventory-" + time.Now().Format("2006-01-02") + ".json",
		Filters:         []runtime.FileFilter{{DisplayName: "JSON (*.json)", Pattern: "*.json"}},
	})
	if err != nil || p == "" {
		return "", err
	}
	b, err := json.MarshalIndent(a.last, "", "  ")
	if err != nil {
		return "", err
	}
	return p, os.WriteFile(p, b, 0o644)
}

// ExportMarkdown creates a Markdown audit report.
func (a *App) ExportMarkdown() (string, error) {
	if a.last == nil {
		return "", errors.New("run a scan first")
	}
	p, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export background audit report (Markdown)",
		DefaultFilename: "background-report-" + time.Now().Format("2006-01-02") + ".md",
		Filters:         []runtime.FileFilter{{DisplayName: "Markdown (*.md)", Pattern: "*.md"}},
	})
	if err != nil || p == "" {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# WinBGM Background Inventory Audit Report\n\n"))
	sb.WriteString(fmt.Sprintf("- **Machine**: %s\n", a.last.Machine))
	sb.WriteString(fmt.Sprintf("- **OS**: %s\n", a.last.OS))
	sb.WriteString(fmt.Sprintf("- **Scanned At**: %s\n", a.last.ScannedAt))
	sb.WriteString(fmt.Sprintf("- **Total Background RAM Usage**: %s\n", a.last.TotalRAMFormatted))
	sb.WriteString(fmt.Sprintf("- **Applications Detected**: %d\n\n", len(a.last.Apps)))

	sb.WriteString("## Detected Applications\n\n")
	sb.WriteString("| Application | Publisher | Launch Behavior | Active | RAM | Components |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, app := range a.last.Apps {
		activeStr := "No"
		if app.Running {
			activeStr = "Yes"
		}
		ramStr := app.RAMFormatted
		if ramStr == "" {
			ramStr = "—"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %d |\n",
			app.Name, app.Publisher, app.Behavior, activeStr, ramStr, len(app.Components)))
	}

	sb.WriteString("\n## Background Components\n\n")
	sb.WriteString("| Name | Kind | Source | Trigger | Enabled | Running | RAM | SHA256 |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")
	for _, app := range a.last.Apps {
		for _, c := range app.Components {
			enStr := "Disabled"
			if c.Enabled {
				enStr = "Enabled"
			}
			runStr := "Stopped"
			if c.Running {
				runStr = "Running"
			}
			ramStr := c.RAMFormatted
			if ramStr == "" {
				ramStr = "—"
			}
			hashStr := c.SHA256
			if len(hashStr) > 12 {
				hashStr = hashStr[:12] + "…"
			} else if hashStr == "" {
				hashStr = "—"
			}
			name := c.Display
			if name == "" {
				name = c.Name
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | `%s` |\n",
				name, c.Kind, c.Source, c.Trigger, enStr, runStr, ramStr, hashStr))
		}
	}

	return p, os.WriteFile(p, []byte(sb.String()), 0o644)
}

// ExportCSV creates a spreadsheet CSV file.
func (a *App) ExportCSV() (string, error) {
	if a.last == nil {
		return "", errors.New("run a scan first")
	}
	p, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export background inventory (CSV)",
		DefaultFilename: "background-inventory-" + time.Now().Format("2006-01-02") + ".csv",
		Filters:         []runtime.FileFilter{{DisplayName: "CSV (*.csv)", Pattern: "*.csv"}},
	})
	if err != nil || p == "" {
		return "", err
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Application", "Component Name", "Kind", "Source", "Trigger", "Enabled", "Running", "RAM", "Signature", "Publisher", "Executable Path", "SHA256"})

	for _, app := range a.last.Apps {
		for _, c := range app.Components {
			name := c.Display
			if name == "" {
				name = c.Name
			}
			_ = w.Write([]string{
				app.Name,
				name,
				c.Kind,
				c.Source,
				c.Trigger,
				fmt.Sprint(c.Enabled),
				fmt.Sprint(c.Running),
				c.RAMFormatted,
				c.Signature,
				c.Publisher,
				c.ExePath,
				c.SHA256,
			})
		}
	}
	w.Flush()

	return p, os.WriteFile(p, buf.Bytes(), 0o644)
}

// GetInstalledApps returns all installed programs on the machine along with aggregate size and counts.
func (a *App) GetInstalledApps() (backend.InstalledAppsResponse, error) {
	apps, summary, err := backend.ScanInstalledApps()
	if err != nil {
		return backend.InstalledAppsResponse{}, err
	}
	a.installedCache = apps
	return backend.InstalledAppsResponse{
		Apps:    apps,
		Summary: summary,
	}, nil
}

func (a *App) findInstalledApp(appID string) (backend.InstalledApp, bool) {
	for _, app := range a.installedCache {
		if app.ID == appID {
			return app, true
		}
	}
	apps, _, err := backend.ScanInstalledApps()
	if err == nil {
		a.installedCache = apps
		for _, app := range apps {
			if app.ID == appID {
				return app, true
			}
		}
	}
	return backend.InstalledApp{}, false
}

// ScanAppResiduals audits leftover directories, heavy models, caches, and registry keys for an app.
func (a *App) ScanAppResiduals(appID string) (backend.ResidualReport, error) {
	if app, ok := a.findInstalledApp(appID); ok {
		return backend.ScanResiduals(app)
	}
	return backend.ScanResiduals(backend.InstalledApp{ID: appID, Name: appID})
}

// LaunchUninstaller triggers the official uninstaller for an app and waits for completion.
func (a *App) LaunchUninstaller(appID string, quiet bool) (int, error) {
	app, ok := a.findInstalledApp(appID)
	if !ok {
		return -1, fmt.Errorf("application %s not found", appID)
	}
	return backend.LaunchUninstaller(app, quiet)
}

// CleanResidualItems safely cleans selected leftover files (to Recycle Bin) and registry keys.
func (a *App) CleanResidualItems(items []backend.ResidualItem) (backend.CleanResult, error) {
	return backend.CleanResidualItems(items)
}

// GetFileVirusTotalURL computes the SHA-256 hash of a file and returns its VirusTotal URL.
func (a *App) GetFileVirusTotalURL(path string) string {
	return backend.FileVirusTotalURL(path)
}

// CreateSystemRestorePoint creates a native Windows System Restore Point.
func (a *App) CreateSystemRestorePoint(description string) (backend.RestorePointResult, error) {
	return backend.CreateSystemRestorePoint(description)
}

// DisableSentinelAlert safely disables a newly added startup entry flagged by Watchdog.
func (a *App) DisableSentinelAlert(id string) (backend.ChangeResult, error) {
	return backend.DisableSentinelAlert(id)
}

// DismissSentinelAlert dismisses a single Watchdog alert.
func (a *App) DismissSentinelAlert(id string) {
	backend.DismissSentinelAlert(id)
}

// GetMinimizeToTray returns whether closing the window minimizes WinBGM to the system tray.
func (a *App) GetMinimizeToTray() bool {
	return a.minimizeToTray
}

// SetMinimizeToTray enables or disables minimizing to the system tray on window close.
func (a *App) SetMinimizeToTray(enabled bool) bool {
	a.minimizeToTray = enabled
	p := trayFlagPath()
	if enabled {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("1"), 0o644)
	} else {
		_ = os.Remove(p)
	}
	return a.minimizeToTray
}

// LiveProcessesResponse wraps grouped running processes and system telemetry for Wails IPC.
type LiveProcessesResponse struct {
	Groups    []backend.ProcessGroup  `json:"groups"`
	Telemetry backend.SystemTelemetry `json:"telemetry"`
}

// GetLiveProcesses returns all running processes grouped by application alongside CPU/RAM/socket telemetry.
func (a *App) GetLiveProcesses() LiveProcessesResponse {
	groups, tel := backend.GetLiveProcessGroups()
	return LiveProcessesResponse{Groups: groups, Telemetry: tel}
}

// SetProcessEcoMode toggles Windows 11 Efficiency Mode (EcoQoS) on a single PID.
func (a *App) SetProcessEcoMode(pid uint32, enable bool) error {
	return backend.SetProcessEcoMode(pid, enable)
}

// SetGroupEcoMode toggles Windows 11 Efficiency Mode (EcoQoS) across all PIDs of an app group.
func (a *App) SetGroupEcoMode(pids []uint32, enable bool) error {
	var lastErr error
	for _, pid := range pids {
		if err := backend.SetProcessEcoMode(pid, enable); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// SetProcessPriority updates the scheduling priority class of a PID.
func (a *App) SetProcessPriority(pid uint32, level string) error {
	return backend.SetProcessPriority(pid, level)
}

// KillProcessTree terminates all PIDs belonging to a process group.
func (a *App) KillProcessTree(pids []uint32) error {
	var lastErr error
	for _, pid := range pids {
		if err := backend.KillProcess(pid); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// FindLockingProcesses queries Windows Restart Manager to identify processes locking a file or folder.
func (a *App) FindLockingProcesses(targetPath string) ([]backend.LockingProcess, error) {
	return backend.FindLockingProcesses(targetPath)
}

// BrowseForLockedFile opens a native Windows file picker dialog for the Lock Finder tool.
func (a *App) BrowseForLockedFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select a locked file to inspect",
	})
}

// GetAppIcons extracts 32x32 PNG icons for a batch of executable or icon paths.
func (a *App) GetAppIcons(paths []string) map[string]string {
	return backend.GetAppIcons(paths)
}

// GetSystemAbout returns hardware, motherboard, storage, and Windows OS details.
func (a *App) GetSystemAbout() backend.SystemAboutInfo {
	return backend.GetSystemAbout()
}

// CreateCustomStartup creates a custom startup entry or delayed logon scheduled task.
func (a *App) CreateCustomStartup(spec backend.CustomStartupSpec) error {
	return backend.CreateCustomStartup(spec)
}

// BrowseStartupExecutable opens a file picker for custom startup executables or scripts.
func (a *App) BrowseStartupExecutable() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select program or script to run at startup",
		Filters: []runtime.FileFilter{
			{DisplayName: "Executables & Scripts (*.exe, *.bat, *.cmd, *.ps1)", Pattern: "*.exe;*.bat;*.cmd;*.ps1;*.vbs;*.lnk"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
}

// CheckForUpdates checks GitHub Releases for a newer version of WinBGM.
func (a *App) CheckForUpdates() backend.UpdateCheckResult {
	return backend.CheckForUpdates()
}

