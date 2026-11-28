package backend

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ============================================================================
// Winget Updates Integration
//
// A read-first updates page built on the Windows Package Manager CLI:
//
//   - Scanning runs `winget upgrade --include-unknown` and only PARSES its
//     output — it changes nothing on the system.
//   - Updating runs `winget upgrade --id <id> --exact --silent` with all
//     agreements pre-accepted and interactivity disabled, so winget never
//     sits waiting on stdin inside a hidden window.
//   - IDs submitted from the UI are validated against a strict charset AND
//     against the last scan's cache before any update is attempted, so a
//     tampered renderer cannot invoke winget with arbitrary arguments
//     (winget is spawned directly, never through a shell, but defense in
//     depth costs nothing).
//   - WinBGM itself is never updated from inside WinBGM: replacing the
//     running executable from the app's own updater is unreliable.
//
// The winget process itself triggers any UAC elevation prompts it needs;
// WinBGM neither requests nor suppresses them.
// ============================================================================

// WingetUpdate is one package that `winget upgrade` reports as upgradable.
type WingetUpdate struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	CurrentVersion   string `json:"currentVersion"`
	AvailableVersion string `json:"availableVersion"`
	Source           string `json:"source"`
	UnknownAvailable bool   `json:"unknownAvailable"` // winget shows "Unknown" for the new version
	Pinned           bool   `json:"pinned"`           // upgrade availability is blocked (e.g. pin)
	Warning          string `json:"warning,omitempty"`
}

// WingetReport is the result of one `winget upgrade` scan.
type WingetReport struct {
	Available     bool           `json:"available"` // winget found on this machine
	WingetVersion string         `json:"wingetVersion"`
	Elevated      bool           `json:"elevated"`
	Updates       []WingetUpdate `json:"updates"`
	Errors        []string       `json:"errors"`
	ScannedAt     string         `json:"scannedAt"`
	DurationMs    int64          `json:"durationMs"`
}

// WingetStatus describes the winget installation itself.
type WingetStatus struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	Path      string `json:"path"`
	Note      string `json:"note,omitempty"`
}

// WingetUpdateResult is the outcome of updating one package.
type WingetUpdateResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	OK         bool   `json:"ok"`
	Message    string `json:"message"`
	OutputTail string `json:"outputTail,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

const (
	wingetScanTimeout = 3 * time.Minute
	wingetVerTimeout  = 20 * time.Second
	wingetUpdTimeout  = 45 * time.Minute // huge installers (IDEs, runtimes) are slow
	wingetOutputTail  = 4000             // chars kept for error reporting
)

// Winget's package IDs are publisher-qualified dotted names ("Git.Git",
// "Microsoft.VCRedist.2015+.x86") or raw Store IDs ("XPDCN05L89WS0C").
var wingetIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+]{0,120}$`)

var (
	wingetCacheMu sync.Mutex
	wingetCache   map[string]WingetUpdate // last scan's results, keyed by lower(ID)
	wingetOpMu    sync.Mutex              // one update at a time (winget locks its own DB anyway)
)

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// GetWingetStatus reports whether winget is usable on this machine.
func GetWingetStatus() WingetStatus {
	p, err := wingetPath()
	if err != nil {
		return WingetStatus{
			Available: false,
			Note:      "winget (Windows Package Manager) was not found. Install the \"App Installer\" package from the Microsoft Store, or get it from https://aka.ms/getwinget.",
		}
	}
	ver := ""
	if out, err := wingetRun(p, wingetVerTimeout, "--version"); err == nil {
		ver = strings.TrimSpace(out)
	}
	return WingetStatus{Available: true, Version: ver, Path: p}
}

// ScanWingetUpdates lists upgradable packages. It is read-only.
func ScanWingetUpdates() WingetReport {
	start := time.Now()
	report := WingetReport{
		Elevated:  isElevated(),
		ScannedAt: time.Now().Format(time.RFC3339),
		Updates:   []WingetUpdate{},
		Errors:    []string{},
	}

	exe, err := wingetPath()
	if err != nil {
		report.Errors = append(report.Errors,
			"winget was not found. Install \"App Installer\" from the Microsoft Store (https://aka.ms/getwinget).")
		report.DurationMs = time.Since(start).Milliseconds()
		return report
	}
	report.Available = true
	if out, err := wingetRun(exe, wingetVerTimeout, "--version"); err == nil {
		report.WingetVersion = strings.TrimSpace(out)
	}

	out, err := wingetRun(exe, wingetScanTimeout,
		"upgrade", "--include-unknown", "--accept-source-agreements", "--disable-interactivity", "--nowarn")
	if err != nil {
		// A missing source agreement or transient source error still carries
		// usable text; surface it but keep an empty (not nil) list.
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = err.Error()
		}
		report.Errors = append(report.Errors, "winget upgrade failed: "+firstLines(msg, 3))
		report.DurationMs = time.Since(start).Milliseconds()
		return report
	}

	updates := parseWingetUpgrades(out)
	for i := range updates {
		if strings.Contains(strings.ToLower(updates[i].ID), "winbgm") {
			updates[i].Warning = "This is WinBGM itself — update it manually (the app must restart to replace its own executable)."
		}
		if updates[i].Pinned {
			updates[i].Warning = joinWarning(updates[i].Warning, "A version pin may block this upgrade.")
		}
		if updates[i].UnknownAvailable {
			updates[i].Warning = joinWarning(updates[i].Warning, "winget cannot determine the available version — the vendor's metadata is incomplete.")
		}
	}

	wingetCacheMu.Lock()
	wingetCache = make(map[string]WingetUpdate, len(updates))
	for _, u := range updates {
		wingetCache[strings.ToLower(u.ID)] = u
	}
	wingetCacheMu.Unlock()

	report.Updates = updates
	report.DurationMs = time.Since(start).Milliseconds()
	return report
}

// UpdateWingetPackage updates exactly one package by ID. The ID must appear in
// the most recent scan and pass the charset check; anything else is rejected.
func UpdateWingetPackage(id string) (WingetUpdateResult, error) {
	id = strings.TrimSpace(id)
	res := WingetUpdateResult{ID: id}

	if !wingetIDRe.MatchString(id) {
		return res, fmt.Errorf("invalid package id")
	}
	wingetCacheMu.Lock()
	cached, ok := wingetCache[strings.ToLower(id)]
	wingetCacheMu.Unlock()
	if !ok {
		return res, fmt.Errorf("package %q is not in the last update scan — scan again before updating", id)
	}
	res.Name = cached.Name
	if cached.Warning != "" && strings.Contains(strings.ToLower(cached.ID), "winbgm") {
		return res, fmt.Errorf("WinBGM cannot update itself while running — update it manually from winget or the app's release page")
	}

	if !wingetOpMu.TryLock() {
		return res, fmt.Errorf("another package update is already in progress")
	}
	defer wingetOpMu.Unlock()

	exe, err := wingetPath()
	if err != nil {
		return res, fmt.Errorf("winget was not found")
	}

	start := time.Now()
	out, err := wingetRun(exe, wingetUpdTimeout,
		"upgrade", "--id", id, "--exact", "--silent",
		"--accept-package-agreements", "--accept-source-agreements",
		"--disable-interactivity", "--nowarn")
	res.DurationMs = time.Since(start).Milliseconds()
	tail := lastLines(out, wingetOutputTail)

	if err != nil {
		res.OK = false
		res.OutputTail = tail
		res.Message = friendlyWingetFailure(id, err, out)
		appendWingetHistory(res, cached, "failed")
		// Return the result (with OK=false) instead of an error: a non-nil
		// error makes Wails reject the JS promise and discard res, which
		// would throw away Message and OutputTail — the details the UI needs
		// to show WHY the update failed.
		return res, nil
	}

	res.OK = true
	res.OutputTail = tail
	res.Message = fmt.Sprintf("Updated %s%s.", orElseStr(cached.Name, id), wingetVersionNote(cached))
	appendWingetHistory(res, cached, "applied")
	return res, nil
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

func wingetPath() (string, error) {
	if p, err := exec.LookPath("winget.exe"); err == nil {
		return p, nil
	}
	// App Execution Aliases sometimes aren't on PATH for GUI processes;
	// check the canonical location directly.
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		p := filepath.Join(localAppData, "Microsoft", "WindowsApps", "winget.exe")
		if fi, err := os.Stat(p); err == nil && fi.Size() > 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("winget not found")
}

// wingetRun executes winget with hidden window and returns trimmed combined output.
func wingetRun(exe string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// parseWingetUpgrades parses `winget upgrade` table output. It prefers exact
// column offsets taken from the (English) header row; if the header can't be
// located (localized output), it falls back to splitting on runs of 2+ spaces.
func parseWingetUpgrades(raw string) []WingetUpdate {
	lines := strings.Split(raw, "\n")

	// Locate the header row and its column offsets.
	var nameEnd, idStart, idEnd, verStart, verEnd, availStart, availEnd, srcStart int
	headerFound := false
	headerIdx := -1
	for i, ln := range lines {
		t := strings.TrimRight(ln, "\r")
		// Header must contain Id, Version and (usually) Source. "Available"
		// only appears when upgrades exist, so don't require it.
		iID := indexWord(t, "Id")
		iVer := strings.Index(t, "Version")
		iAvail := strings.Index(t, "Available")
		iSrc := strings.Index(t, "Source")
		if iID <= 0 || iVer < iID {
			continue
		}
		if iSrc > iVer || iAvail > iVer {
			idStart, idEnd = iID, minNonNeg(iVer, len(t))
			verStart = iVer
			if iAvail > iVer {
				verEnd, availStart = iAvail, iAvail
				if iSrc > iAvail {
					availEnd, srcStart = iSrc, iSrc
				} else {
					availEnd, srcStart = len(t), -1
				}
			} else if iSrc > iVer {
				verEnd, availStart, availEnd = iSrc, -1, -1
				srcStart = iSrc
			} else {
				verEnd = len(t)
			}
			nameEnd = idStart
			headerFound = true
			headerIdx = i
			break
		}
	}

	var updates []WingetUpdate
	seen := map[string]bool{}
	for li, ln := range lines {
		line := strings.TrimRight(ln, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || isWingetNoiseLine(trimmed) || li == headerIdx {
			continue
		}

		var name, id, ver, avail, src string
		if headerFound {
			// winget expands columns to fit content, so every data field stays
			// column-aligned: the id column must start on a non-space preceded
			// by a space. Short banner lines sliced at header offsets would
			// otherwise produce nonsense tokens.
			if idStart >= len(line) || line[idStart] == ' ' || (idStart > 0 && line[idStart-1] != ' ') {
				continue
			}
			name = columnSlice(line, 0, nameEnd)
			id = columnSlice(line, idStart, idEnd)
			ver = columnSlice(line, verStart, verEnd)
			if availStart >= 0 {
				avail = columnSlice(line, availStart, availEnd)
			}
			if srcStart >= 0 {
				src = columnSlice(line, srcStart, len(line))
			}
		} else {
			fields := wingetFieldSplit(trimmed)
			if len(fields) < 4 {
				continue
			}
			name, id, ver, avail = fields[0], fields[1], fields[2], fields[3]
			if len(fields) >= 5 {
				src = fields[4]
			}
		}

		if !wingetIDRe.MatchString(id) {
			continue // headers, summaries, banners, pinned notices
		}
		if strings.EqualFold(avail, "unknown") {
			// keep: --include-unknown is asked for explicitly
		} else if avail == "" {
			continue // no upgrade information in this row
		}

		u := WingetUpdate{
			ID:               id,
			Name:             strings.TrimSpace(name),
			CurrentVersion:   strings.TrimSpace(ver),
			AvailableVersion: strings.TrimSpace(avail),
			Source:           strings.TrimSpace(src),
			UnknownAvailable: strings.EqualFold(avail, "unknown"),
			Pinned:           strings.EqualFold(avail, "blocked"),
		}
		if u.Name == "" {
			u.Name = u.ID
		}
		key := strings.ToLower(u.ID)
		if seen[key] {
			continue
		}
		seen[key] = true
		updates = append(updates, u)
	}

	sort.SliceStable(updates, func(i, j int) bool { return updates[i].Name < updates[j].Name })
	if updates == nil {
		updates = []WingetUpdate{}
	}
	return updates
}

// indexWord finds a standalone "Id" token (not a substring like "Builder").
func indexWord(s, w string) int {
	for i := 0; i+len(w) <= len(s); i++ {
		if s[i:i+len(w)] != w {
			continue
		}
		before := byte(' ')
		after := byte(' ')
		if i > 0 {
			before = s[i-1]
		}
		if i+len(w) < len(s) {
			after = s[i+len(w)]
		}
		if before == ' ' && after == ' ' {
			return i
		}
	}
	return -1
}

func minNonNeg(a, b int) int {
	if a < 0 {
		return b
	}
	if b < 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func columnSlice(line string, start, end int) string {
	if start < 0 || start >= len(line) {
		return ""
	}
	if end > len(line) {
		end = len(line)
	}
	if end <= start {
		return ""
	}
	return strings.TrimSpace(line[start:end])
}

var wingetMultiSpaceRe = regexp.MustCompile(`\s{2,}`)

func wingetFieldSplit(s string) []string {
	return wingetMultiSpaceRe.Split(strings.TrimSpace(s), -1)
}

func isWingetNoiseLine(t string) bool {
	if strings.Trim(t, "-") == "" { // separator rule
		return true
	}
	// Progress spinners / table footer / summary banners.
	if strings.HasPrefix(t, "-") || strings.HasPrefix(t, "\\") || strings.HasPrefix(t, "|") {
		return true
	}
	low := strings.ToLower(t)
	if strings.Contains(low, "upgrades available") ||
		strings.Contains(low, "upgrade(s) available") ||
		strings.HasPrefix(low, "the following packages") ||
		strings.Contains(low, "newer version of winget") ||
		strings.Contains(low, "an update to winget is available") {
		return true
	}
	return false
}

func friendlyWingetFailure(id string, err error, out string) string {
	tail := lastLines(out, 600)
	tail = strings.TrimSpace(tail)
	low := strings.ToLower(out)
	switch {
	case strings.Contains(low, "no installed package found") || strings.Contains(low, "no package found matching"):
		return fmt.Sprintf("winget no longer knows the package %s (it may have been uninstalled). Rescan and try again.", id)
	case strings.Contains(low, "multiple packages found matching"):
		return "winget matched several packages for this id — rescan and update by its exact id from the table."
	case strings.Contains(low, "installer hash mismatch"):
		return "The vendor's installer failed winget's hash check (possibly an interrupted mirror). Rescan and retry later."
	case strings.Contains(low, "requires elevation") || strings.Contains(low, "elevation is required"):
		return "This package needs administrator rights. Restart WinBGM as administrator (Settings) and try again."
	case strings.Contains(low, "user cancelled") || strings.Contains(low, "canceled by user"):
		return "The update was cancelled (a UAC prompt was dismissed)."
	case strings.Contains(low, "cannot be upgraded") || strings.Contains(low, "pinned"):
		return "This package is pinned or cannot be upgraded by winget. Update it from inside the app itself."
	}
	if tail == "" {
		return fmt.Sprintf("winget exited with an error: %v", err)
	}
	return fmt.Sprintf("winget exited with an error: %s", firstLines(tail, 3))
}

func wingetVersionNote(cached WingetUpdate) string {
	if cached.CurrentVersion == "" || cached.AvailableVersion == "" ||
		cached.UnknownAvailable || strings.EqualFold(cached.AvailableVersion, "unknown") {
		return ""
	}
	return fmt.Sprintf(" (%s → %s)", cached.CurrentVersion, cached.AvailableVersion)
}

func appendWingetHistory(res WingetUpdateResult, cached WingetUpdate, status string) {
	_ = appendHistory(HistoryEntry{
		ID:      fmt.Sprintf("winget-%s-%d", sanitizeHistoryKey(res.ID), time.Now().UnixNano()),
		Time:    time.Now().Format(time.RFC3339),
		Title:   orElseStr(cached.Name, "Winget package update"),
		AppName: orElseStr(cached.Name, res.ID),
		Kind:    "winget",
		Action:  "update",
		Summary: res.Message,
		Before:  cached.CurrentVersion,
		After:   ternaryStr(res.OK, cached.AvailableVersion, cached.CurrentVersion),
		Status:  status,
		Error:   ternaryStr(res.OK, "", res.Message),
		// Package updates are installer-level state changes; WinBGM cannot
		// roll back an arbitrary installer, and the Recycle Bin model does
		// not apply to program binaries.
		Restorable: false,
	})
}

func sanitizeHistoryKey(id string) string {
	b := []byte(strings.ToLower(id))
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-') {
			b[i] = '_'
		}
	}
	return string(b)
}

// --- tiny string helpers -----------------------------------------------------

func orElseStr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func ternaryStr(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func joinWarning(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + " " + add
}

func firstLines(s string, n int) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	out := strings.Join(lines, " ")
	return strings.TrimSpace(out)
}

func lastLines(s string, maxChars int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxChars {
		return s
	}
	return s[len(s)-maxChars:]
}
