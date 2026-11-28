package main

import (
	"fmt"

	"winbgm/backend"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// UninstallerService exposes program uninstallation, junk cleaner, and Winget update methods to Wails.
type UninstallerService struct {
	app *App
}

// NewUninstallerService creates a domain service backed by App.
func NewUninstallerService(app *App) *UninstallerService {
	return &UninstallerService{app: app}
}

// GetInstalledApps returns all installed programs on the machine along with aggregate size and counts.
func (s *UninstallerService) GetInstalledApps() (backend.InstalledAppsResponse, error) {
	apps, summary, err := backend.ScanInstalledApps()
	if err != nil {
		return backend.InstalledAppsResponse{}, err
	}
	s.app.installedCache = apps
	return backend.InstalledAppsResponse{
		Apps:    apps,
		Summary: summary,
	}, nil
}

func (s *UninstallerService) findInstalledApp(appID string) (backend.InstalledApp, bool) {
	for _, app := range s.app.installedCache {
		if app.ID == appID {
			return app, true
		}
	}
	apps, _, err := backend.ScanInstalledApps()
	if err == nil {
		s.app.installedCache = apps
		for _, app := range apps {
			if app.ID == appID {
				return app, true
			}
		}
	}
	return backend.InstalledApp{}, false
}

// ScanAppResiduals audits leftover directories, heavy models, caches, and registry keys for an app.
func (s *UninstallerService) ScanAppResiduals(appID string) (backend.ResidualReport, error) {
	if app, ok := s.findInstalledApp(appID); ok {
		return backend.ScanResiduals(app)
	}
	return backend.ScanResiduals(backend.InstalledApp{ID: appID, Name: appID})
}

// LaunchUninstaller triggers the official uninstaller for an app and waits for completion.
func (s *UninstallerService) LaunchUninstaller(appID string, quiet bool) (int, error) {
	app, ok := s.findInstalledApp(appID)
	if !ok {
		return -1, fmt.Errorf("application %s not found", appID)
	}
	return backend.LaunchUninstaller(app, quiet)
}

// CleanResidualItems safely cleans selected leftover files (to Recycle Bin) and registry keys.
func (s *UninstallerService) CleanResidualItems(items []backend.ResidualItem) (backend.CleanResult, error) {
	return backend.CleanResidualItems(items)
}

// ScanJunk audits system-wide junk and caches. It changes nothing.
func (s *UninstallerService) ScanJunk() backend.JunkReport {
	return backend.ScanJunk()
}

// CleanJunkItems cleans selected junk items and streams live "junk_progress" events.
func (s *UninstallerService) CleanJunkItems(items []backend.JunkItem) (backend.CleanResult, error) {
	backend.SetJunkProgressFunc(func(p backend.JunkProgress) {
		runtime.EventsEmit(s.app.ctx, "junk_progress", p)
	})
	defer backend.SetJunkProgressFunc(nil)
	return backend.CleanJunkItems(items)
}

// GetWingetStatus reports whether the Windows Package Manager (winget) is available.
func (s *UninstallerService) GetWingetStatus() backend.WingetStatus {
	return backend.GetWingetStatus()
}

// ScanWingetUpdates lists upgradable packages via `winget upgrade`. Read-only.
func (s *UninstallerService) ScanWingetUpdates() backend.WingetReport {
	return backend.ScanWingetUpdates()
}

// UpdateWingetPackage updates one package via `winget upgrade --id ... --exact --silent`.
func (s *UninstallerService) UpdateWingetPackage(id string) (backend.WingetUpdateResult, error) {
	return backend.UpdateWingetPackage(id)
}
