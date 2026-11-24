package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// ResidualCategory defines the grouping of leftover items.
type ResidualCategory string

const (
	CategoryHeavy     ResidualCategory = "heavy"     // AI models, SDKs, massive caches
	CategoryAppData   ResidualCategory = "appdata"   // AppData / LocalAppData / ProgramData
	CategoryInstall   ResidualCategory = "install"   // Main installation directory remnants
	CategoryRegistry  ResidualCategory = "registry"  // Orphaned registry keys
	CategoryAutostart ResidualCategory = "autostart" // Dead Run keys or background entries
)

// ResidualItem represents a single file, folder, or registry key leftover from an application.
type ResidualItem struct {
	ID            string           `json:"id"`
	Category      ResidualCategory `json:"category"`
	CategoryTitle string           `json:"categoryTitle"`
	Title         string           `json:"title"`
	Path          string           `json:"path"`
	SizeBytes     uint64           `json:"sizeBytes"`
	SizeFormatted string           `json:"sizeFormatted"`
	ItemCount     int              `json:"itemCount"`
	Description   string           `json:"description"`
	IsSelected    bool             `json:"isSelected"`
	IsRegistry    bool             `json:"isRegistry"`
}

// ResidualReport contains the complete audit of leftover files, caches, and registry data for an app.
type ResidualReport struct {
	AppID              string         `json:"appId"`
	AppName            string         `json:"appName"`
	TotalSizeBytes     uint64         `json:"totalSizeBytes"`
	TotalSizeFormatted string         `json:"totalSizeFormatted"`
	HeavySizeBytes     uint64         `json:"heavySizeBytes"`
	HeavySizeFormatted string         `json:"heavySizeFormatted"`
	Items              []ResidualItem `json:"items"`
}

// CleanResult reports the outcome of a cleanup operation.
type CleanResult struct {
	FreedBytes          uint64   `json:"freedBytes"`
	FreedSizeFormatted  string   `json:"freedSizeFormatted"`
	DeletedFilesCount   int      `json:"deletedFilesCount"`
	DeletedKeysCount    int      `json:"deletedKeysCount"`
	Errors              []string `json:"errors"`
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// sanitizeName extracts clean tokens for path discovery.
func sanitizeName(name string) []string {
	clean := strings.ToLower(name)
	var tokens []string
	tokens = append(tokens, clean)

	// Strip version numbers e.g. "CapCut 9.6" -> "CapCut"
	tokens = append(tokens, nonAlphanumericRegex.ReplaceAllString(clean, ""))

	parts := strings.Fields(clean)
	if len(parts) > 1 {
		tokens = append(tokens, parts[0])
	}
	return tokens
}

// ScanResiduals performs an in-depth audit of leftover files, directories, models, and registry keys for an app.
func ScanResiduals(app InstalledApp) (ResidualReport, error) {
	report := ResidualReport{
		AppID:   app.ID,
		AppName: app.Name,
	}

	seenPaths := make(map[string]bool)
	var items []ResidualItem

	// 1. Check Smart Heavy Dependency Profiles (e.g. Ollama models, Android SDK, VS Code extensions)
	profiles := FindMatchingProfiles(app.Name, app.ID)
	for _, prof := range profiles {
		for _, ps := range prof.PathSpecs {
			expanded := ExpandPath(ps)
			if seenPaths[strings.ToLower(expanded)] {
				continue
			}

			if fi, err := os.Stat(expanded); err == nil {
				seenPaths[strings.ToLower(expanded)] = true
				var bytes uint64
				var count int
				if fi.IsDir() {
					bytes, count = ComputeFolderSize(expanded, 20000)
				} else {
					bytes = uint64(fi.Size())
					count = 1
				}

				if bytes > 0 || count > 0 {
					item := ResidualItem{
						ID:            fmt.Sprintf("heavy_%d", len(items)),
						Category:      CategoryHeavy,
						CategoryTitle: prof.Category,
						Title:         prof.Title,
						Path:          expanded,
						SizeBytes:     bytes,
						SizeFormatted: FormatBytes(bytes),
						ItemCount:     count,
						Description:   prof.Description,
						IsSelected:    true,
						IsRegistry:    false,
					}
					items = append(items, item)
					report.HeavySizeBytes += bytes
				}
			}
		}
	}

	// 2. Check AppData, LocalAppData, and ProgramData directories
	candidateBases := []struct {
		base     string
		catTitle string
	}{
		{os.Getenv("LOCALAPPDATA"), "Local AppData Cache & Storage"},
		{os.Getenv("APPDATA"), "Roaming AppData Configuration"},
		{os.Getenv("ProgramData"), "System ProgramData Storage"},
	}

	nameTokens := sanitizeName(app.Name)
	if app.Publisher != "" {
		nameTokens = append(nameTokens, strings.ToLower(app.Publisher))
	}

	for _, cb := range candidateBases {
		if cb.base == "" {
			continue
		}
		for _, tok := range nameTokens {
			if len(tok) < 3 {
				continue
			}
			targetDir := filepath.Join(cb.base, tok)
			lowTarget := strings.ToLower(targetDir)
			if seenPaths[lowTarget] {
				continue
			}

			if fi, err := os.Stat(targetDir); err == nil && fi.IsDir() {
				seenPaths[lowTarget] = true
				bytes, count := ComputeFolderSize(targetDir, 5000)
				if bytes > 0 || count > 0 {
					items = append(items, ResidualItem{
						ID:            fmt.Sprintf("appdata_%d", len(items)),
						Category:      CategoryAppData,
						CategoryTitle: cb.catTitle,
						Title:         fmt.Sprintf("%s Folder (%s)", filepath.Base(cb.base), filepath.Base(targetDir)),
						Path:          targetDir,
						SizeBytes:     bytes,
						SizeFormatted: FormatBytes(bytes),
						ItemCount:     count,
						Description:   "Application data, configuration files, local databases, and temporary caches.",
						IsSelected:    true,
						IsRegistry:    false,
					})
				}
			}
		}
	}

	// 3. Check InstallLocation remnants (if uninstaller didn't wipe directory completely)
	if app.InstallLocation != "" {
		loc := app.InstallLocation
		lowLoc := strings.ToLower(loc)
		if !seenPaths[lowLoc] {
			if fi, err := os.Stat(loc); err == nil && fi.IsDir() {
				seenPaths[lowLoc] = true
				bytes, count := ComputeFolderSize(loc, 5000)
				if bytes > 0 || count > 0 {
					items = append(items, ResidualItem{
						ID:            fmt.Sprintf("install_%d", len(items)),
						Category:      CategoryInstall,
						CategoryTitle: "Installation Directory Remnants",
						Title:         "Leftover Program Files",
						Path:          loc,
						SizeBytes:     bytes,
						SizeFormatted: FormatBytes(bytes),
						ItemCount:     count,
						Description:   "Main installation folder containing leftover binaries, libraries, or uninstaller logs.",
						IsSelected:    true,
						IsRegistry:    false,
					})
				}
			}
		}
	}

	// 4. Check Registry remnants
	regRoots := []struct {
		root     registry.Key
		rootName string
	}{
		{registry.CURRENT_USER, "HKCU"},
		{registry.LOCAL_MACHINE, "HKLM"},
	}

	for _, rr := range regRoots {
		for _, tok := range nameTokens {
			if len(tok) < 3 {
				continue
			}
			subPath := `Software\` + tok
			fullKey := rr.rootName + `\` + subPath
			lowKey := strings.ToLower(fullKey)
			if seenPaths[lowKey] {
				continue
			}

			k, err := registry.OpenKey(rr.root, subPath, registry.READ)
			if err == nil {
				seenPaths[lowKey] = true
				subKeys, _ := k.ReadSubKeyNames(0)
				valNames, _ := k.ReadValueNames(0)
				k.Close()

				items = append(items, ResidualItem{
					ID:            fmt.Sprintf("reg_%d", len(items)),
					Category:      CategoryRegistry,
					CategoryTitle: "Registry Configurations",
					Title:         fmt.Sprintf("%s Settings Key", rr.rootName),
					Path:          fullKey,
					SizeBytes:     0,
					SizeFormatted: fmt.Sprintf("%d values, %d subkeys", len(valNames), len(subKeys)),
					ItemCount:     len(valNames) + len(subKeys),
					Description:   "Application preferences, licensing tokens, file associations, and window positions.",
					IsSelected:    true,
					IsRegistry:    true,
				})
			}
		}
	}

	// 5. Check dead Run / Autostart keys
	runRoots := []struct {
		root     registry.Key
		rootName string
		path     string
	}{
		{registry.CURRENT_USER, "HKCU", `Software\Microsoft\Windows\CurrentVersion\Run`},
		{registry.LOCAL_MACHINE, "HKLM", `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`},
	}
	for _, rr := range runRoots {
		k, err := registry.OpenKey(rr.root, rr.path, registry.READ)
		if err == nil {
			names, _ := k.ReadValueNames(0)
			for _, n := range names {
				lowN := strings.ToLower(n)
				cmd, _, _ := k.GetStringValue(n)
				lowCmd := strings.ToLower(cmd)

				matched := false
				for _, tok := range nameTokens {
					if strings.Contains(lowN, tok) || strings.Contains(lowCmd, tok) {
						matched = true
						break
					}
				}

				if matched {
					regPath := fmt.Sprintf(`%s\%s\%s`, rr.rootName, rr.path, n)
					items = append(items, ResidualItem{
						ID:            fmt.Sprintf("autostart_%d", len(items)),
						Category:      CategoryAutostart,
						CategoryTitle: "Autostart Remnants",
						Title:         fmt.Sprintf("Orphaned Run Key: %s", n),
						Path:          regPath,
						SizeBytes:     0,
						SizeFormatted: "Autostart Entry",
						ItemCount:     1,
						Description:   fmt.Sprintf("Startup command launching: %s", cmd),
						IsSelected:    true,
						IsRegistry:    true,
					})
				}
			}
			k.Close()
		}
	}

	for _, it := range items {
		report.TotalSizeBytes += it.SizeBytes
	}
	report.TotalSizeFormatted = FormatBytes(report.TotalSizeBytes)
	report.HeavySizeFormatted = FormatBytes(report.HeavySizeBytes)
	report.Items = items

	return report, nil
}

// CleanResidualItems safely deletes the selected residual files (to Recycle Bin) and registry keys.
func CleanResidualItems(items []ResidualItem) (CleanResult, error) {
	var result CleanResult

	for _, item := range items {
		if !item.IsSelected {
			continue
		}

		if item.IsRegistry {
			// Registry deletion
			if item.Category == CategoryAutostart {
				// Dead run key: parse parent and value name
				parts := strings.Split(item.Path, `\`)
				if len(parts) >= 3 {
					valName := parts[len(parts)-1]
					keyPath := strings.Join(parts[1:len(parts)-1], `\`)
					var root registry.Key
					if strings.EqualFold(parts[0], "HKCU") {
						root = registry.CURRENT_USER
					} else {
						root = registry.LOCAL_MACHINE
					}
					k, err := registry.OpenKey(root, keyPath, registry.SET_VALUE)
					if err == nil {
						k.DeleteValue(valName)
						k.Close()
						result.DeletedKeysCount++
					}
				}
			} else {
				// Whole registry key
				err := SafeDeleteRegistryKey(item.Path)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Registry error on %s: %v", item.Path, err))
				} else {
					result.DeletedKeysCount++
				}
			}
		} else {
			// File or directory deletion: Send to Recycle Bin!
			err := SafeRecyclePath(item.Path)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("File error on %s: %v", item.Path, err))
			} else {
				result.FreedBytes += item.SizeBytes
				result.DeletedFilesCount += item.ItemCount
			}
		}
	}

	result.FreedSizeFormatted = FormatBytes(result.FreedBytes)
	return result, nil
}
