package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	CategoryShortcut  ResidualCategory = "shortcut"  // Start Menu & Desktop .lnk shortcuts
	CategoryCrashDump ResidualCategory = "crashdump" // CrashDumps & WER reports
)

// ResidualItem represents a single file, folder, or registry key leftover from an application.
type ResidualItem struct {
	ID                string           `json:"id"`
	Category          ResidualCategory `json:"category"`
	CategoryTitle     string           `json:"categoryTitle"`
	Title             string           `json:"title"`
	Path              string           `json:"path"`
	SizeBytes         uint64           `json:"sizeBytes"`
	SizeFormatted     string           `json:"sizeFormatted"`
	ItemCount         int              `json:"itemCount"`
	Description       string           `json:"description"`
	IsSelected        bool             `json:"isSelected"`
	IsRegistry        bool             `json:"isRegistry"`
	ConfidenceScore   int              `json:"confidenceScore"`
	ConfidenceLevel   string           `json:"confidenceLevel"` // "Very High", "Good", "Questionable", "Low"
	ConfidenceReasons []string         `json:"confidenceReasons"`
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
	FreedBytes         uint64   `json:"freedBytes"`
	FreedSizeFormatted string   `json:"freedSizeFormatted"`
	DeletedFilesCount  int      `json:"deletedFilesCount"`
	DeletedKeysCount   int      `json:"deletedKeysCount"`
	Errors             []string `json:"errors"`
}

var (
	residualCacheMu sync.Mutex
	residualCache   = make(map[string]ResidualItem)
	residualScanSeq uint64
)

var versionSuffixRegex = regexp.MustCompile(`(?i)\s+(v?\d+(\.\d+)+|\(\s*x(64|86)\s*\)|\d+-bit).*$`)

var genericVendorTokens = map[string]bool{
	"microsoft": true, "windows": true, "google": true, "adobe": true,
	"intel": true, "nvidia": true, "amd": true, "apple": true,
	"oracle": true, "python": true, "java": true, "asus": true,
	"dell": true, "lenovo": true, "hp": true, "razer": true,
	"logitech": true, "common": true, "system": true, "update": true,
	"updater": true, "launcher": true, "desktop": true, "helper": true,
	"service": true, "client": true, "runtime": true, "redistributable": true,
	"program": true, "software": true, "app": true, "tools": true,
}

func sanitizeSubpathSegment(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "..", "")
	s = strings.ReplaceAll(s, `/`, " ")
	s = strings.ReplaceAll(s, `\`, " ")
	return strings.TrimSpace(s)
}

func isBroadRootDir(dir string) bool {
	clean := strings.ToLower(filepath.Clean(strings.TrimSpace(dir)))
	if clean == "" || len(clean) <= 3 {
		return true
	}
	broad := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("ProgramW6432"),
		os.Getenv("SystemRoot"),
		os.Getenv("LOCALAPPDATA"),
		os.Getenv("APPDATA"),
		os.Getenv("ProgramData"),
		os.Getenv("USERPROFILE"),
	}
	for _, b := range broad {
		if b != "" && strings.EqualFold(clean, filepath.Clean(b)) {
			return true
		}
	}
	return false
}

func pathUnderDir(target, dir string) bool {
	if strings.TrimSpace(dir) == "" || isBroadRootDir(dir) {
		return false
	}
	t := strings.ToLower(filepath.Clean(target))
	d := strings.ToLower(filepath.Clean(dir))
	return t == d || strings.HasPrefix(t, d+`\`)
}

func isAppStillInstalled(app InstalledApp) bool {
	if app.ExePath != "" {
		if _, err := os.Stat(app.ExePath); err == nil {
			return true
		}
	}
	if app.UninstallString != "" && !strings.EqualFold(app.Source, "Orphaned") {
		if exe := exePath(app.UninstallString); exe != "" {
			if _, err := os.Stat(exe); err == nil {
				return true
			}
		}
	}
	return false
}

// candidateAppSubpaths builds strict, non-generic folder/registry subpaths for an app.
// It never guesses using a bare first word or generic vendor name.
func candidateAppSubpaths(app InstalledApp) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || strings.Contains(s, "..") {
			return
		}
		low := strings.ToLower(s)
		if genericVendorTokens[low] || protectedFolderNames[low] {
			return
		}
		if len(s) < 4 && !strings.EqualFold(s, sanitizeSubpathSegment(app.Name)) {
			return
		}
		if !seen[low] {
			seen[low] = true
			out = append(out, s)
		}
	}

	rawName := sanitizeSubpathSegment(app.Name)
	add(rawName)

	stripped := sanitizeSubpathSegment(versionSuffixRegex.ReplaceAllString(rawName, ""))
	add(stripped)

	pub := sanitizeSubpathSegment(app.Publisher)
	if pub != "" && rawName != "" && !strings.EqualFold(pub, rawName) {
		add(filepath.Join(pub, rawName))
		if stripped != "" && stripped != rawName {
			add(filepath.Join(pub, stripped))
		}
	}

	return out
}

// ScanResiduals performs an in-depth audit of leftover files, directories, models, and registry keys for an app.
// All items default to IsSelected: false so the user must explicitly opt-in before any cleanup.
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
			if err := ValidateCleanPath(expanded); err != nil {
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
						IsSelected:    false,
						IsRegistry:    false,
					}
					items = append(items, item)
					report.HeavySizeBytes += bytes
				}
			}
		}
	}

	// 2. Check AppData, LocalAppData, and ProgramData directories (strict subpaths only)
	candidateBases := []struct {
		base     string
		catTitle string
	}{
		{os.Getenv("LOCALAPPDATA"), "Local AppData Cache & Storage"},
		{os.Getenv("APPDATA"), "Roaming AppData Configuration"},
		{os.Getenv("ProgramData"), "System ProgramData Storage"},
	}

	subpaths := candidateAppSubpaths(app)

	for _, cb := range candidateBases {
		if cb.base == "" {
			continue
		}
		for _, sub := range subpaths {
			targetDir := filepath.Join(cb.base, sub)
			lowTarget := strings.ToLower(targetDir)
			if seenPaths[lowTarget] {
				continue
			}
			if err := ValidateCleanPath(targetDir); err != nil {
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
						IsSelected:    false,
						IsRegistry:    false,
					})
				}
			}
		}
	}

	// 3. Check InstallLocation remnants
	if app.InstallLocation != "" {
		loc := app.InstallLocation
		lowLoc := strings.ToLower(loc)
		if !seenPaths[lowLoc] && ValidateCleanPath(loc) == nil {
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
						IsSelected:    false,
						IsRegistry:    false,
					})
				}
			}
		}
	}

	// 4. Check Registry remnants (strict subpaths only)
	regRoots := []struct {
		root     registry.Key
		rootName string
	}{
		{registry.CURRENT_USER, "HKCU"},
		{registry.LOCAL_MACHINE, "HKLM"},
	}

	for _, rr := range regRoots {
		for _, sub := range subpaths {
			subPath := `Software\` + sub
			fullKey := rr.rootName + `\` + subPath
			lowKey := strings.ToLower(fullKey)
			if seenPaths[lowKey] {
				continue
			}
			if _, _, err := ValidateCleanRegistryPath(fullKey); err != nil {
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
					IsSelected:    false,
					IsRegistry:    true,
				})
			}
		}
	}

	// 5. Check dead Run / Autostart keys (strict path or exact app-name match only)
	runRoots := []struct {
		root     registry.Key
		rootName string
		path     string
	}{
		{registry.CURRENT_USER, "HKCU", `Software\Microsoft\Windows\CurrentVersion\Run`},
		{registry.LOCAL_MACHINE, "HKLM", `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`},
	}
	lowInstall := strings.ToLower(app.InstallLocation)
	lowExe := strings.ToLower(app.ExePath)
	exactName := strings.ToLower(strings.TrimSpace(app.Name))

	for _, rr := range runRoots {
		k, err := registry.OpenKey(rr.root, rr.path, registry.READ)
		if err == nil {
			names, _ := k.ReadValueNames(0)
			for _, n := range names {
				lowN := strings.ToLower(n)
				cmd, _, _ := k.GetStringValue(n)

				cmdExe := exePath(cmd)
				matched := false
				if lowExe != "" && cmdExe != "" && pathUnderDir(cmdExe, app.ExePath) {
					matched = true
				} else if lowInstall != "" && cmdExe != "" && pathUnderDir(cmdExe, app.InstallLocation) {
					matched = true
				} else if exactName != "" && !genericVendorTokens[exactName] && len(exactName) >= 4 && strings.EqualFold(lowN, exactName) {
					matched = true
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
						IsSelected:    false,
						IsRegistry:    true,
					})
				}
			}
			k.Close()
		}
	}

	// 6. BCU-inspired ShortcutJunk: Start Menu & Desktop .lnk files pointing to InstallLocation or ExePath
	if (lowInstall != "" && len(lowInstall) > 6) || lowExe != "" {
		shortcutDirs := []string{
			filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`),
			filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`),
			filepath.Join(os.Getenv("USERPROFILE"), `Desktop`),
		}
		for _, sd := range shortcutDirs {
			if sd == "" {
				continue
			}
			_ = filepath.Walk(sd, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(p), ".lnk") {
					return nil
				}
				if seenPaths[strings.ToLower(p)] {
					return nil
				}
				target, _ := resolveShortcut(p)
				if target == "" {
					return nil
				}
				lowT := strings.ToLower(target)
				if (lowExe != "" && lowT == lowExe) || pathUnderDir(target, app.InstallLocation) {
					seenPaths[strings.ToLower(p)] = true
					items = append(items, ResidualItem{
						ID:            fmt.Sprintf("lnk_%d", len(items)),
						Category:      CategoryShortcut,
						CategoryTitle: "Shortcuts (.lnk)",
						Title:         fmt.Sprintf("Shortcut: %s", filepath.Base(p)),
						Path:          p,
						SizeBytes:     uint64(info.Size()),
						SizeFormatted: FormatBytes(uint64(info.Size())),
						ItemCount:     1,
						Description:   fmt.Sprintf("Shortcut pointing to %s", target),
						IsSelected:    false,
						IsRegistry:    false,
					})
				}
				return nil
			})
		}
	}

	// 7. BCU-inspired WerScanner: CrashDumps matching the app's executable
	if lowExe != "" {
		exeBase := strings.ToLower(filepath.Base(lowExe))
		crashDir := filepath.Join(os.Getenv("LOCALAPPDATA"), "CrashDumps")
		if dumps, err := filepath.Glob(filepath.Join(crashDir, exeBase+".*.dmp")); err == nil {
			for _, dmp := range dumps {
				if fi, err := os.Stat(dmp); err == nil && !fi.IsDir() {
					items = append(items, ResidualItem{
						ID:            fmt.Sprintf("dmp_%d", len(items)),
						Category:      CategoryCrashDump,
						CategoryTitle: "Crash Dumps",
						Title:         filepath.Base(dmp),
						Path:          dmp,
						SizeBytes:     uint64(fi.Size()),
						SizeFormatted: FormatBytes(uint64(fi.Size())),
						ItemCount:     1,
						Description:   "Windows crash dump generated by this application.",
						IsSelected:    false,
						IsRegistry:    false,
					})
				}
			}
		}
	}

	scanSeq := atomic.AddUint64(&residualScanSeq, 1)
	stillInstalled := isAppStillInstalled(app)
	for i := range items {
		items[i].ID = fmt.Sprintf("s%d_%s", scanSeq, items[i].ID)
		scoreResidualConfidence(app, &items[i], stillInstalled)
		report.TotalSizeBytes += items[i].SizeBytes
	}
	report.TotalSizeFormatted = FormatBytes(report.TotalSizeBytes)
	report.HeavySizeFormatted = FormatBytes(report.HeavySizeBytes)
	report.Items = items

	// Cache authoritative items in Go so CleanResidualItems never trusts UI-supplied paths
	residualCacheMu.Lock()
	for _, it := range items {
		residualCache[it.ID] = it
	}
	residualCacheMu.Unlock()

	return report, nil
}

// scoreResidualConfidence assigns a BCU-inspired ConfidenceScore and ConfidenceLevel to each leftover item.
func scoreResidualConfidence(app InstalledApp, item *ResidualItem, stillInstalled bool) {
	score := 0
	var reasons []string

	pub := strings.TrimSpace(app.Publisher)
	lowPath := strings.ToLower(item.Path)

	switch item.Category {
	case CategoryHeavy:
		score += 8
		reasons = append(reasons, "Matched curated application dependency profile (+8)")
	case CategoryAutostart:
		score += 7
		reasons = append(reasons, "Explicitly referenced by app path or exact Run key (+7)")
	case CategoryShortcut:
		score += 7
		reasons = append(reasons, "Shortcut target points directly to uninstalled app (+7)")
	case CategoryCrashDump:
		score += 6
		reasons = append(reasons, "Crash dump matches app executable name (+6)")
	case CategoryInstall:
		score += 5
		reasons = append(reasons, "Matches registry InstallLocation (+5)")
		if item.ItemCount == 0 {
			score += 4
			reasons = append(reasons, "Folder is empty (+4)")
		}
	default:
		// AppData or Registry
		stripped := strings.TrimSpace(versionSuffixRegex.ReplaceAllString(app.Name, ""))
		baseItem := filepath.Base(item.Path)
		if strings.EqualFold(baseItem, strings.TrimSpace(app.Name)) || (stripped != "" && strings.EqualFold(baseItem, stripped)) {
			score += 4
			reasons = append(reasons, "Exact product name match (+4)")
		} else {
			score += 1
			reasons = append(reasons, "Partial name match (+1)")
		}

		if pub != "" && len(pub) >= 3 && strings.Contains(lowPath, `\`+strings.ToLower(pub)+`\`) {
			score += 4
			reasons = append(reasons, "Nested under matching Publisher folder (+4)")
		} else if pub != "" && !strings.EqualFold(pub, app.Name) {
			score -= 1
			reasons = append(reasons, "Directly under shared root without Publisher folder (-1)")
		}

		if item.ItemCount == 0 && !item.IsRegistry {
			score += 3
			reasons = append(reasons, "Directory is empty (+3)")
		}
	}

	if stillInstalled && (item.Category == CategoryInstall || item.Category == CategoryAutostart || item.Category == CategoryShortcut) {
		score -= 6
		reasons = append(reasons, "Application still appears installed (-6)")
	}

	item.ConfidenceScore = score
	item.ConfidenceReasons = reasons
	switch {
	case score >= 7:
		item.ConfidenceLevel = "Very High"
	case score >= 3:
		item.ConfidenceLevel = "Good"
	case score >= 0:
		item.ConfidenceLevel = "Questionable"
	default:
		item.ConfidenceLevel = "Low"
	}
}

// CleanResidualItems safely deletes the selected residual files (to Recycle Bin) and registry keys.
// It looks up every requested item by ID from the backend scan cache and validates its path before touching disk or registry.
func CleanResidualItems(items []ResidualItem) (CleanResult, error) {
	var result CleanResult

	for _, reqItem := range items {
		if !reqItem.IsSelected {
			continue
		}

		// Look up authoritative item from backend cache; never trust paths from the UI
		residualCacheMu.Lock()
		authItem, ok := residualCache[reqItem.ID]
		residualCacheMu.Unlock()
		if !ok || !strings.EqualFold(authItem.Path, reqItem.Path) {
			result.Errors = append(result.Errors, fmt.Sprintf("Rejected unverified cleanup item %q: please rescan first", reqItem.ID))
			continue
		}

		if authItem.IsRegistry {
			if authItem.Category == CategoryAutostart {
				parts := strings.Split(authItem.Path, `\`)
				if len(parts) >= 3 {
					valName := parts[len(parts)-1]
					keyPath := strings.Join(parts[1:len(parts)-1], `\`)
					var root registry.Key
					if strings.EqualFold(parts[0], "HKCU") {
						root = registry.CURRENT_USER
					} else {
						root = registry.LOCAL_MACHINE
					}

					// Back up the Run key before removing the value
					parentKeyPath := strings.Join(parts[:len(parts)-1], `\`)
					backupFile, err := exportRegistryBackup(parentKeyPath)
					if err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("Aborted Run-key removal (%s): backup failed: %v", authItem.Path, err))
						continue
					}

					k, err := registry.OpenKey(root, keyPath, registry.SET_VALUE)
					if err == nil {
						if delErr := k.DeleteValue(valName); delErr != nil {
							result.Errors = append(result.Errors, fmt.Sprintf("Registry value error on %s: %v", authItem.Path, delErr))
						} else {
							result.DeletedKeysCount++
							_ = appendHistory(HistoryEntry{
								ID:         fmt.Sprintf("runclean-%d", time.Now().UnixNano()),
								Time:       time.Now().Format(time.RFC3339),
								Title:      "Removed orphaned Run entry: " + valName,
								Summary:    fmt.Sprintf("Backed up Run key to %s before removing %q", backupFile, valName),
								Status:     "applied",
								Restorable: false,
							})
						}
						k.Close()
					}
				}
			} else {
				err := SafeDeleteRegistryKey(authItem.Path)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("Registry error on %s: %v", authItem.Path, err))
				} else {
					result.DeletedKeysCount++
				}
			}
		} else {
			err := SafeRecyclePath(authItem.Path)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Recycle Bin error on %s: %v", authItem.Path, err))
			} else {
				result.FreedBytes += authItem.SizeBytes
				result.DeletedFilesCount += authItem.ItemCount
			}
		}
	}

	result.FreedSizeFormatted = FormatBytes(result.FreedBytes)
	return result, nil
}
