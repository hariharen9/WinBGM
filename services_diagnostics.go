package main

import (
	"winbgm/backend"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// DiagnosticsService exposes snapshots, boot/power diagnostics, file lock finder, and system specs to Wails.
type DiagnosticsService struct {
	app *App
}

// NewDiagnosticsService creates a domain service backed by App.
func NewDiagnosticsService(app *App) *DiagnosticsService {
	return &DiagnosticsService{app: app}
}

// SaveSnapshot saves a baseline snapshot to disk.
func (s *DiagnosticsService) SaveSnapshot(name string) (backend.SnapshotMeta, error) {
	if s.app.last == nil {
		inv := backend.BuildInventory()
		s.app.last = &inv
	}
	return backend.SaveSnapshot(name, *s.app.last)
}

// ListSnapshots returns all saved baseline snapshots.
func (s *DiagnosticsService) ListSnapshots() ([]backend.SnapshotMeta, error) {
	return backend.ListSnapshots()
}

// DeleteSnapshot removes a snapshot.
func (s *DiagnosticsService) DeleteSnapshot(id string) error {
	return backend.DeleteSnapshot(id)
}

// DiffSnapshots compares a saved baseline snapshot against the live inventory.
func (s *DiagnosticsService) DiffSnapshots(baseID string) (backend.SnapshotDiff, error) {
	if s.app.last == nil {
		inv := backend.BuildInventory()
		s.app.last = &inv
	}
	return backend.DiffSnapshots(baseID, s.app.last)
}

// GetBootProfile returns boot duration metrics and app boot degradation delays from Windows Diagnostics.
func (s *DiagnosticsService) GetBootProfile() backend.BootProfile {
	return backend.GetBootProfile()
}

// GetPowerReport checks active sleep and display turn-off blocker requests.
func (s *DiagnosticsService) GetPowerReport() backend.PowerReport {
	return backend.GetPowerReport()
}

// FindLockingProcesses queries Windows Restart Manager to identify processes locking a file or folder.
func (s *DiagnosticsService) FindLockingProcesses(targetPath string) ([]backend.LockingProcess, error) {
	return backend.FindLockingProcesses(targetPath)
}

// BrowseForLockedFile opens a native Windows file picker dialog for the Lock Finder tool.
func (s *DiagnosticsService) BrowseForLockedFile() (string, error) {
	return runtime.OpenFileDialog(s.app.ctx, runtime.OpenDialogOptions{
		Title: "Select a locked file to inspect",
	})
}

// GetSystemAbout returns hardware, motherboard, storage, and Windows OS details.
func (s *DiagnosticsService) GetSystemAbout() backend.SystemAboutInfo {
	return backend.GetSystemAbout()
}
