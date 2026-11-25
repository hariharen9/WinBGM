package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// InstalledApp represents an installed software package discovered from the Windows registry.
type InstalledApp struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Version             string `json:"version"`
	Publisher           string `json:"publisher"`
	InstallDate         string `json:"installDate"`
	EstimatedSizeKB     uint64 `json:"estimatedSizeKB"`
	SizeBytes           uint64 `json:"sizeBytes"`
	DisplaySize         string `json:"displaySize"`
	InstallLocation     string `json:"installLocation"`
	UninstallString     string `json:"uninstallString"`
	QuietUninstall      string `json:"quietUninstall"`
	DisplayIcon         string `json:"displayIcon"`
	RegistryPath        string `json:"registryPath"`
	IsUserInstall       bool   `json:"isUserInstall"`
	Is64Bit             bool   `json:"is64Bit"`
	IsSystemComponent   bool   `json:"isSystemComponent"`
	ExePath             string `json:"exePath"`
	Source              string `json:"source"`        // "Win32", "Scoop", "Steam"
	InstallerType       string `json:"installerType"` // "MSI", "InnoSetup", "NSIS", "Squirrel", "InstallShield", "Winget", "Standard", "Scoop", "Steam"
	HasHeavyProfile     bool   `json:"hasHeavyProfile"`
	HeavyProfileKey     string `json:"heavyProfileKey"`
	HeavyEstimatedBytes uint64 `json:"heavyEstimatedBytes"`
	HeavyDisplaySize    string `json:"heavyDisplaySize"`
}

// UninstallerSummary provides aggregate statistics on installed software.
type UninstallerSummary struct {
	TotalApps          int    `json:"totalApps"`
	TotalSizeBytes     uint64 `json:"totalSizeBytes"`
	TotalSizeFormatted string `json:"totalSizeFormatted"`
	HeavyAppsCount     int    `json:"heavyAppsCount"`
	UserAppsCount      int    `json:"userAppsCount"`
	SystemAppsCount    int    `json:"systemAppsCount"`
}

// InstalledAppsResponse wraps the installed app list and summary for clean Wails IPC serialization.
type InstalledAppsResponse struct {
	Apps    []InstalledApp     `json:"apps"`
	Summary UninstallerSummary `json:"summary"`
}

// FormatBytes formats a byte count into a clean human-readable string.
func FormatBytes(bytes uint64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
		tb = 1024 * gb
	)
	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(tb))
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	case bytes > 0:
		return fmt.Sprintf("%d B", bytes)
	default:
		return "—"
	}
}

type uninstRoot struct {
	root     registry.Key
	path     string
	isUser   bool
	is64Bit  bool
	rootName string
}

// ScanInstalledApps scans the Windows registry for installed applications.
func ScanInstalledApps() ([]InstalledApp, UninstallerSummary, error) {
	roots := []uninstRoot{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, false, true, "HKLM"},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`, false, false, "HKLM_32"},
		{registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Uninstall`, true, true, "HKCU"},
	}

	seen := make(map[string]bool)
	var apps []InstalledApp
	var summary UninstallerSummary

	for _, r := range roots {
		k, err := registry.OpenKey(r.root, r.path, registry.READ)
		if err != nil {
			continue
		}
		subnames, err := k.ReadSubKeyNames(0)
		k.Close()
		if err != nil {
			continue
		}

		for _, subname := range subnames {
			appKeyPath := r.path + `\` + subname
			subKey, err := registry.OpenKey(r.root, appKeyPath, registry.READ)
			if err != nil {
				continue
			}

			displayName, _, _ := subKey.GetStringValue("DisplayName")
			displayName = strings.TrimSpace(displayName)
			if displayName == "" {
				subKey.Close()
				continue
			}

			// Deduplicate identical apps across roots if same display name & version
			displayVersion, _, _ := subKey.GetStringValue("DisplayVersion")
			dedupKey := strings.ToLower(displayName + "_" + displayVersion)
			if seen[dedupKey] {
				subKey.Close()
				continue
			}
			seen[dedupKey] = true

			publisher, _, _ := subKey.GetStringValue("Publisher")
			installDate, _, _ := subKey.GetStringValue("InstallDate")
			installLoc, _, _ := subKey.GetStringValue("InstallLocation")
			uninstStr, _, _ := subKey.GetStringValue("UninstallString")
			quietUninst, _, _ := subKey.GetStringValue("QuietUninstallString")
			displayIcon, _, _ := subKey.GetStringValue("DisplayIcon")

			// Check if marked as system component
			sysComp, _, _ := subKey.GetIntegerValue("SystemComponent")
			parentKeyName, _, _ := subKey.GetStringValue("ParentKeyName")
			isSys := (sysComp == 1) || (parentKeyName != "")

			// Read EstimatedSize (stored as DWORD in KB)
			sizeKB, _, _ := subKey.GetIntegerValue("EstimatedSize")
			subKey.Close()

			sizeBytes := sizeKB * 1024

			// Clean InstallDate e.g. "20260530" -> "2026-05-30"
			formattedDate := installDate
			if len(installDate) == 8 && !strings.Contains(installDate, "-") && !strings.Contains(installDate, "/") {
				formattedDate = fmt.Sprintf("%s-%s-%s", installDate[0:4], installDate[4:6], installDate[6:8])
			}

			// Extract primary executable path
			exe := extractExePath(displayIcon, installLoc, uninstStr)

			// Check for smart heavy dependency profiles (e.g. Ollama models, Android SDK, VS Code extensions)
			profiles := FindMatchingProfiles(displayName, subname)
			var hasHeavy bool
			var heavyKey string

			if len(profiles) > 0 {
				for _, prof := range profiles {
					for _, ps := range prof.PathSpecs {
						expanded := ExpandPath(ps)
						if _, err := os.Stat(expanded); err == nil {
							hasHeavy = true
							heavyKey = prof.Key
							break
						}
					}
					if hasHeavy {
						break
					}
				}
			}

			cleanedLoc := strings.Trim(installLoc, `" `)
			if cleanedLoc == "" && exe != "" {
				dir := filepath.Dir(exe)
				baseDir := strings.ToLower(filepath.Base(dir))
				if baseDir != "system32" && baseDir != "syswow64" && baseDir != "windows" &&
					baseDir != "programs" && baseDir != "program files" && baseDir != "program files (x86)" {
					cleanedLoc = dir
				}
			}

			instKind, genQuiet := DetectInstallerType(strings.TrimSpace(uninstStr), strings.TrimSpace(quietUninst))

			app := InstalledApp{
				ID:                  r.rootName + "_" + subname,
				Name:                displayName,
				Version:             displayVersion,
				Publisher:           strings.TrimSpace(publisher),
				InstallDate:         formattedDate,
				EstimatedSizeKB:     sizeKB,
				SizeBytes:           sizeBytes,
				DisplaySize:         FormatBytes(sizeBytes),
				InstallLocation:     cleanedLoc,
				UninstallString:     strings.TrimSpace(uninstStr),
				QuietUninstall:      genQuiet,
				DisplayIcon:         displayIcon,
				RegistryPath:        r.rootName + `\` + appKeyPath,
				IsUserInstall:       r.isUser,
				Is64Bit:             r.is64Bit,
				IsSystemComponent:   isSys,
				ExePath:             exe,
				Source:              "Win32",
				InstallerType:       instKind,
				HasHeavyProfile:     hasHeavy,
				HeavyProfileKey:     heavyKey,
				HeavyEstimatedBytes: 0,
				HeavyDisplaySize:    "—",
			}

			apps = append(apps, app)
			summary.TotalApps++
			summary.TotalSizeBytes += sizeBytes
			if hasHeavy {
				summary.HeavyAppsCount++
			}
			if r.isUser {
				summary.UserAppsCount++
			}
			if isSys {
				summary.SystemAppsCount++
			}
		}
	}

	// Merge Scoop and Steam packages (deduplicating against Win32 entries by name)
	seenNames := make(map[string]bool, len(apps))
	for _, a := range apps {
		seenNames[strings.ToLower(a.Name)] = true
	}
	for _, extra := range append(scanScoopApps(), scanSteamApps()...) {
		if seenNames[strings.ToLower(extra.Name)] {
			continue
		}
		seenNames[strings.ToLower(extra.Name)] = true
		apps = append(apps, extra)
		summary.TotalApps++
		summary.TotalSizeBytes += extra.SizeBytes
		if extra.IsUserInstall {
			summary.UserAppsCount++
		}
	}

	// Sort apps by size descending by default
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].SizeBytes != apps[j].SizeBytes {
			return apps[i].SizeBytes > apps[j].SizeBytes
		}
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})

	summary.TotalSizeFormatted = FormatBytes(summary.TotalSizeBytes)
	return apps, summary, nil
}

// extractExePath attempts to find the primary executable path for an app from icon, location, or uninstall string.
func extractExePath(displayIcon, installLoc, uninstStr string) string {
	// Try display icon first (e.g. "C:\Program Files\App\app.exe,0")
	if displayIcon != "" {
		cleaned := strings.Trim(displayIcon, `" `)
		if idx := strings.Index(cleaned, ","); idx > 0 {
			cleaned = cleaned[:idx]
		}
		if filepath.Ext(cleaned) == ".exe" {
			if _, err := os.Stat(cleaned); err == nil {
				return cleaned
			}
		}
	}

	// Try InstallLocation
	if installLoc != "" {
		cleanedLoc := strings.Trim(installLoc, `" `)
		// Check for executables in installLoc root
		if entries, err := os.ReadDir(cleanedLoc); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".exe") {
					candidate := filepath.Join(cleanedLoc, e.Name())
					if !strings.Contains(strings.ToLower(e.Name()), "unins") &&
						!strings.Contains(strings.ToLower(e.Name()), "setup") {
						return candidate
					}
				}
			}
		}
	}

	// Try uninstStr
	if uninstStr != "" {
		p := strings.Trim(uninstStr, `" `)
		if strings.HasPrefix(p, `"`) {
			if end := strings.Index(p[1:], `"`); end > 0 {
				p = p[1 : end+1]
			}
		} else if sp := strings.Index(p, " "); sp > 0 {
			p = p[:sp]
		}
		if filepath.Ext(p) == ".exe" {
			return p
		}
	}

	return ""
}

// FileVirusTotalURL computes the SHA-256 hash of a binary and returns its VirusTotal GUI URL.
func FileVirusTotalURL(path string) string {
	if path == "" {
		return ""
	}
	if h := calcSHA256(path); h != "" {
		return "https://www.virustotal.com/gui/file/" + h
	}
	return ""
}
