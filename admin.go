package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const compatLayersKey = `Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`

func getAppExePaths() []string {
	var paths []string
	seen := map[string]bool{}
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[strings.ToLower(p)] {
			return
		}
		seen[strings.ToLower(p)] = true
		paths = append(paths, p)
	}

	if exe, err := os.Executable(); err == nil && exe != "" {
		add(exe)
		dir := filepath.Dir(exe)
		add(filepath.Join(dir, "BackgroundManager.exe"))
		add(filepath.Join(dir, "build", "bin", "BackgroundManager.exe"))
	}
	if cwd, err := os.Getwd(); err == nil {
		add(filepath.Join(cwd, "build", "bin", "BackgroundManager.exe"))
	}
	return paths
}

func isCompatRunAsAdmin(root registry.Key, path, exe string) bool {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	val, _, err := k.GetStringValue(exe)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToUpper(val), "RUNASADMIN")
}

func checkAlwaysAdmin() bool {
	for _, p := range getAppExePaths() {
		if isCompatRunAsAdmin(registry.CURRENT_USER, compatLayersKey, p) ||
			isCompatRunAsAdmin(registry.LOCAL_MACHINE, compatLayersKey, p) {
			return true
		}
	}
	return false
}

// GetAlwaysAdmin returns true if the app executable has the RUNASADMIN compatibility flag set.
func (a *App) GetAlwaysAdmin() bool {
	return checkAlwaysAdmin()
}

// SetAlwaysAdmin enables or disables the RUNASADMIN compatibility flag in Windows registry (HKCU).
func (a *App) SetAlwaysAdmin(enable bool) (bool, error) {
	paths := getAppExePaths()
	if len(paths) == 0 {
		return false, errors.New("could not determine executable path")
	}

	k, _, err := registry.CreateKey(registry.CURRENT_USER, compatLayersKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()

	for _, p := range paths {
		if enable {
			if err := k.SetStringValue(p, "~ RUNASADMIN"); err != nil {
				return false, err
			}
		} else {
			if err := k.DeleteValue(p); err != nil && !errors.Is(err, registry.ErrNotExist) {
				return false, err
			}
		}
	}
	return enable, nil
}
