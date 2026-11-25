package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

var innoFilenameRe = regexp.MustCompile(`(?i)unins\d{3}$`)

// DetectInstallerType identifies the installer framework (MSI, InnoSetup, NSIS, Squirrel, InstallShield, Winget, etc.)
// and synthesizes a reliable silent/quiet uninstall command when the vendor omitted QuietUninstallString.
func DetectInstallerType(uninstStr, quietStr string) (kind string, generatedQuiet string) {
	u := strings.TrimSpace(uninstStr)
	q := strings.TrimSpace(quietStr)
	if u == "" && q == "" {
		return "Unknown", q
	}
	check := u
	if check == "" {
		check = q
	}
	low := strings.ToLower(check)

	// 1. MSI / Windows Installer
	if strings.Contains(low, "msiexec") || (strings.Contains(low, `\package cache\{`) && strings.Contains(low, ".exe")) {
		kind = "MSI"
		if q == "" && strings.Contains(low, "msiexec") {
			// Replace /I with /X if needed and append /qn /norestart
			msiCmd := regexp.MustCompile(`(?i)\s+/i\b`).ReplaceAllString(u, " /X")
			if !strings.Contains(strings.ToLower(msiCmd), "/qn") && !strings.Contains(strings.ToLower(msiCmd), "/quiet") {
				q = msiCmd + " /qn /norestart"
			}
		}
		return kind, q
	}

	// 2. Winget
	if strings.HasPrefix(low, "winget ") || strings.Contains(low, "winget.exe") {
		kind = "Winget"
		if q == "" && !strings.Contains(low, "--silent") {
			q = u + " --silent"
		}
		return kind, q
	}

	// 3. Squirrel / Electron Update.exe
	if strings.Contains(low, "update.exe") && strings.Contains(low, "--uninstall") {
		kind = "Squirrel"
		if q == "" && !strings.Contains(low, " -s") {
			q = u + " -s"
		}
		return kind, q
	}

	// 4. InstallShield
	if strings.Contains(low, `installshield installation information\{`) {
		return "InstallShield", q
	}

	// Extract uninstaller executable path for binary inspection
	exe := extractExePath("", "", check)
	if exe != "" {
		if _, err := os.Stat(exe); err == nil {
			noExt := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
			// 5. Inno Setup: unins000.exe + unins000.dat
			if innoFilenameRe.MatchString(noExt) {
				datPath := strings.TrimSuffix(exe, filepath.Ext(exe)) + ".dat"
				if _, err := os.Stat(datPath); err == nil {
					kind = "InnoSetup"
					if q == "" {
						q = fmt.Sprintf(`"%s" /VERYSILENT /SUPPRESSMSGBOXES /NORESTART`, exe)
					}
					return kind, q
				}
			}

			// 6. NSIS: check PE header/first 256KB for "Nullsoft" signature (fast bounded read)
			if isNsisBinary(exe) {
				kind = "NSIS"
				if q == "" {
					q = fmt.Sprintf(`"%s" /S`, exe)
				}
				return kind, q
			}
		}
	}

	return "Standard", q
}

func isNsisBinary(exePath string) bool {
	f, err := os.Open(exePath)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 256*1024)
	n, _ := f.Read(buf)
	if n <= 0 {
		return false
	}
	return bytes.Contains(buf[:n], []byte("Nullsoft"))
}

// scanScoopApps discovers packages managed by Scoop (inspired by BCU's ScoopFactory).
func scanScoopApps() []InstalledApp {
	var out []InstalledApp
	var roots []string

	if sp := os.Getenv("SCOOP"); sp != "" {
		roots = append(roots, sp)
	} else if up := os.Getenv("USERPROFILE"); up != "" {
		roots = append(roots, filepath.Join(up, "scoop"))
	}
	if sg := os.Getenv("SCOOP_GLOBAL"); sg != "" {
		roots = append(roots, sg)
	} else if pd := os.Getenv("ProgramData"); pd != "" {
		roots = append(roots, filepath.Join(pd, "scoop"))
	}

	type scoopManifest struct {
		Version     string `json:"version"`
		Description string `json:"description"`
		Homepage    string `json:"homepage"`
	}

	seen := make(map[string]bool)
	for _, root := range roots {
		appsDir := filepath.Join(root, "apps")
		entries, err := os.ReadDir(appsDir)
		if err != nil {
			continue
		}
		isGlobal := strings.Contains(strings.ToLower(root), "programdata")
		for _, e := range entries {
			if !e.IsDir() || strings.EqualFold(e.Name(), "scoop") {
				continue
			}
			appName := e.Name()
			if seen[strings.ToLower(appName)] {
				continue
			}
			curDir := filepath.Join(appsDir, appName, "current")
			manifestPath := filepath.Join(curDir, "manifest.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				continue
			}
			seen[strings.ToLower(appName)] = true

			var m scoopManifest
			_ = json.Unmarshal(data, &m)

			exe := extractExePath("", curDir, "")
			cmd := fmt.Sprintf("scoop uninstall %s", appName)
			if isGlobal {
				cmd = fmt.Sprintf("scoop uninstall -g %s", appName)
			}

			out = append(out, InstalledApp{
				ID:              "Scoop_" + appName,
				Name:            appName,
				Version:         m.Version,
				Publisher:       "Scoop Package",
				InstallLocation: curDir,
				UninstallString: cmd,
				QuietUninstall:  cmd,
				Source:          "Scoop",
				InstallerType:   "Scoop",
				IsUserInstall:   !isGlobal,
				Is64Bit:         true,
				ExePath:         exe,
				DisplaySize:     "—",
				HeavyDisplaySize: "—",
			})
		}
	}
	return out
}

// scanSteamApps discovers installed Steam games via Steam's appmanifest_*.acf files (inspired by BCU's SteamFactory).
func scanSteamApps() []InstalledApp {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.READ)
	if err != nil {
		return nil
	}
	steamPath, _, err := k.GetStringValue("SteamPath")
	k.Close()
	if err != nil || steamPath == "" {
		return nil
	}
	steamPath = filepath.Clean(steamPath)

	libDirs := []string{filepath.Join(steamPath, "steamapps")}
	// Also parse libraryfolders.vdf for additional Steam library drives
	if vdfBytes, err := os.ReadFile(filepath.Join(steamPath, "steamapps", "libraryfolders.vdf")); err == nil {
		rePath := regexp.MustCompile(`"path"\s+"([^"]+)"`)
		for _, m := range rePath.FindAllStringSubmatch(string(vdfBytes), -1) {
			p := strings.ReplaceAll(m[1], `\\`, `\`)
			lib := filepath.Join(p, "steamapps")
			if !strings.EqualFold(lib, libDirs[0]) {
				libDirs = append(libDirs, lib)
			}
		}
	}

	var out []InstalledApp
	seenID := make(map[string]bool)
	kvRe := regexp.MustCompile(`"([^"]+)"\s+"([^"]*)"`)

	for _, lib := range libDirs {
		matches, _ := filepath.Glob(filepath.Join(lib, "appmanifest_*.acf"))
		for _, acfPath := range matches {
			raw, err := os.ReadFile(acfPath)
			if err != nil {
				continue
			}
			kv := make(map[string]string)
			for _, m := range kvRe.FindAllStringSubmatch(string(raw), -1) {
				kv[strings.ToLower(m[1])] = m[2]
			}
			appID := kv["appid"]
			name := kv["name"]
			if appID == "" || name == "" || seenID[appID] {
				continue
			}
			// Skip Steamworks Common Redistributables
			if appID == "228980" {
				continue
			}
			seenID[appID] = true

			var sizeBytes uint64
			if s, err := strconv.ParseUint(kv["sizeondisk"], 10, 64); err == nil {
				sizeBytes = s
			}
			installDir := ""
			if kv["installdir"] != "" {
				installDir = filepath.Join(lib, "common", kv["installdir"])
			}
			exe := extractExePath("", installDir, "")

			out = append(out, InstalledApp{
				ID:               "Steam_" + appID,
				Name:             name,
				Publisher:        "Steam",
				EstimatedSizeKB:  sizeBytes / 1024,
				SizeBytes:        sizeBytes,
				DisplaySize:      FormatBytes(sizeBytes),
				InstallLocation:  installDir,
				UninstallString:  "steam://uninstall/" + appID,
				Source:           "Steam",
				InstallerType:    "Steam",
				Is64Bit:          true,
				ExePath:          exe,
				HeavyDisplaySize: "—",
			})
		}
	}
	return out
}
