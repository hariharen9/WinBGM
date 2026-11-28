package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ============================================================================
// Junk & Cache Cleaner
//
// A system-wide cleanup tool built on the same safety model as the residual
// engine: nothing is ever pre-selected, regenerable junk is deleted
// permanently by default (the only mode that actually frees space; the UI may
// downgrade to a Recycle Bin move per clean), registry keys are exported to
// .reg backups first, and the backend never trusts paths from the UI: every
// cleanup request is verified against the scan's internal cache before
// anything is touched.
// ============================================================================

// JunkCategory groups cleanup targets by what they are and how safe they are.
type JunkCategory string

const (
	JunkCatTemp       JunkCategory = "temp"
	JunkCatUpdate     JunkCategory = "update"
	JunkCatBrowser    JunkCategory = "browser"
	JunkCatShader     JunkCategory = "shader"
	JunkCatThumbs     JunkCategory = "thumbnails"
	JunkCatLogs       JunkCategory = "logs"
	JunkCatWindowsOld JunkCategory = "windowsold"
	JunkCatRecycleBin JunkCategory = "recyclebin"
	JunkCatOrphanReg  JunkCategory = "orphanreg"
)

var junkCategoryOrder = map[JunkCategory]int{
	JunkCatTemp: 0, JunkCatBrowser: 1, JunkCatUpdate: 2, JunkCatShader: 3,
	JunkCatThumbs: 4, JunkCatLogs: 5, JunkCatWindowsOld: 6,
	JunkCatRecycleBin: 7, JunkCatOrphanReg: 8,
}

var junkCategoryTitles = map[JunkCategory]string{
	JunkCatTemp:       "Temporary Files",
	JunkCatBrowser:    "Browser Caches",
	JunkCatUpdate:     "Windows Update Leftovers",
	JunkCatShader:     "GPU Shader Caches",
	JunkCatThumbs:     "Explorer Thumbnail & Icon Caches",
	JunkCatLogs:       "Error Reports & Crash Dumps",
	JunkCatWindowsOld: "Previous Windows Installation",
	JunkCatRecycleBin: "Recycle Bin",
	JunkCatOrphanReg:  "Orphaned Uninstall Entries",
}

// JunkItem is one cleanable target. IsSelected is always false from a scan;
// the user opts in explicitly.
type JunkItem struct {
	ID            string       `json:"id"`
	Category      JunkCategory `json:"category"`
	CategoryTitle string       `json:"categoryTitle"`
	Title         string       `json:"title"`
	Path          string       `json:"path"`
	SizeBytes     uint64       `json:"sizeBytes"`
	SizeFormatted string       `json:"sizeFormatted"`
	ItemCount     int          `json:"itemCount"`
	Description   string       `json:"description"`
	IsSelected    bool         `json:"isSelected"`
	RequiresAdmin bool         `json:"requiresAdmin"`
	IsRegistry    bool         `json:"isRegistry"`
	Warning       string       `json:"warning,omitempty"`
	// UseRecycleBin is the one field the UI may set, and it can only make the
	// cleanup MORE conservative: the backend's per-category policy is
	// "permanent" for regenerable junk (temp/caches/dumps — recycling frees no
	// space until the bin is emptied), and the UI may downgrade that to a
	// Recycle Bin move, never the reverse.
	UseRecycleBin bool `json:"useRecycleBin,omitempty"`
	// DeletionMode mirrors what the backend will actually do for this item
	// ("permanent" or "recycle-bin") so the UI can label rows honestly.
	DeletionMode string `json:"deletionMode,omitempty"`

	contentsOnly bool          // clean children of Path, keep the directory itself
	minAge       time.Duration // skip entries modified more recently than this
	matchPattern func(name string) bool
}

// junkDeletionModes records the safety policy per file category. Every junk
// category holds regenerable artifacts (caches, temp files, dumps, old
// installs), so the default is permanent deletion — that is what frees disk
// space immediately and what Windows' own Disk Cleanup / Storage Sense do.
// The UI can still send UseRecycleBin to get a recoverable Recycle Bin move.
var junkDeletionModes = map[JunkCategory]string{
	JunkCatTemp:       "permanent",
	JunkCatBrowser:    "permanent",
	JunkCatUpdate:     "permanent",
	JunkCatShader:     "permanent",
	JunkCatThumbs:     "permanent",
	JunkCatLogs:       "permanent",
	JunkCatWindowsOld: "permanent",
	JunkCatRecycleBin: "permanent", // emptying the bin is permanent by definition
}

// JunkReport is the full system-wide audit result.
type JunkReport struct {
	Items              []JunkItem `json:"items"`
	TotalSizeBytes     uint64     `json:"totalSizeBytes"`
	TotalSizeFormatted string     `json:"totalSizeFormatted"`
	ScannableCount     int        `json:"scannableCount"`
	ScannedAt          string     `json:"scannedAt"`
	DurationMs         int64      `json:"durationMs"`
	Elevated           bool       `json:"elevated"`
}

var (
	junkCacheMu sync.Mutex
	junkCache   = make(map[string]JunkItem)
)

// Sensitive-name denylist: even though cache paths are enumerated explicitly
// by name, this is defense in depth. Only the final path component (the thing
// actually being cleaned) is matched, so a parent container like
// "...\User Data\Default\Cache" stays eligible while anything named like
// credentials, cookies, or history is never offered for cleanup.
var junkSensitiveTokens = []string{
	"cookies", "login", "logins", "password", "history", "session",
	"bookmark", "credential", "indexeddb", "localstorage", "keychain",
}

func junkPathIsSensitive(path string) bool {
	final := strings.ToLower(filepath.Base(strings.Trim(path, `\`)))
	for _, tok := range junkSensitiveTokens {
		if strings.Contains(final, tok) {
			return true
		}
	}
	return false
}

const (
	junkMaxChildrenPerItem = 5000
	tempMinAge             = 24 * time.Hour
	logsMinAge             = 7 * 24 * time.Hour
)

// ============================================================================
// Scanning
// ============================================================================

func localAppData() string   { return os.Getenv("LOCALAPPDATA") }
func roamingAppData() string { return os.Getenv("APPDATA") }

func systemRoot() string {
	if s := os.Getenv("SystemRoot"); s != "" {
		return s
	}
	return `C:\Windows`
}

// junkCacheDir returns dir if it exists, "" otherwise.
func junkCacheDir(dir string) string {
	if dir == "" {
		return ""
	}
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return dir
	}
	return ""
}

type junkPathSpec struct {
	path  string
	title string
}

// junkProfileCacheDirs returns all existing cache directories inside each
// existing profile directory ("Default", "Profile NNN") of a browser
// "User Data" root.
func junkProfileCacheDirs(userDataBase string, profileNames []string, cacheNames []string, browserLabel string) []junkPathSpec {
	var out []junkPathSpec
	ud, err := os.ReadDir(userDataBase)
	if err != nil {
		return nil
	}
	for _, e := range ud {
		isProfile := false
		for _, pn := range profileNames {
			if pn == "*" {
				if strings.HasPrefix(strings.ToLower(e.Name()), "profile") {
					isProfile = true
				}
			} else if strings.EqualFold(e.Name(), pn) {
				isProfile = true
			}
		}
		if !isProfile || !e.IsDir() {
			continue
		}
		for _, cn := range cacheNames {
			p := filepath.Join(userDataBase, e.Name(), cn)
			if junkCacheDir(p) == "" {
				continue
			}
			out = append(out, junkPathSpec{
				path:  p,
				title: fmt.Sprintf("%s cache (%s)", browserLabel, e.Name()),
			})
		}
	}
	return out
}

var procGetFileAttributesW = modKernel32Instance.NewProc("GetFileAttributesW")

// isReparsePoint reports whether the entry is a symlink or junction, so the
// cleaner never deletes through a link into someone else's real files.
func isReparsePoint(path string, info os.FileInfo) bool {
	if w32data, ok := info.Sys().(*windows.Win32FileAttributeData); ok {
		return w32data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
	}
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return true // Fail closed: can't verify, treat as a link.
	}
	attrs, _, _ := procGetFileAttributesW.Call(uintptr(unsafe.Pointer(ptr)))
	if attrs == 0xFFFFFFFF {
		return true // Stat failed: fail closed.
	}
	return attrs&0x00000400 != 0 // FILE_ATTRIBUTE_REPARSE_POINT
}

// measureJunkItem measures what a cleanup of this item would free, using the
// exact same top-level-children semantics the cleaner uses, so scan numbers
// match what cleaning actually frees.
func measureJunkItem(item JunkItem) (uint64, int) {
	if !item.contentsOnly {
		if item.IsRegistry {
			return 0, 1
		}
		fi, err := os.Stat(item.Path)
		if err != nil {
			return 0, 0
		}
		if !fi.IsDir() {
			return uint64(fi.Size()), 1
		}
		// Whole-directory target: count everything inside, skipping reparse points.
		var bytes uint64
		var count int
		_ = filepath.WalkDir(item.Path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if count >= junkMaxChildrenPerItem {
				return filepath.SkipAll
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if info, err := d.Info(); err == nil && !isReparsePoint(p, info) {
				bytes += uint64(info.Size())
				count++
			}
			return nil
		})
		return bytes, count
	}

	children := eligibleJunkChildren(item.Path, item.minAge, item.matchPattern)
	var bytes uint64
	var count int
	for _, child := range children {
		fi, err := os.Stat(child)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			b, c := ComputeFolderSize(child, 20000)
			bytes += b
			count += c
		} else {
			bytes += uint64(fi.Size())
			count++
		}
	}
	return bytes, count
}

// eligibleJunkChildren lists the top-level entries of dir that cleanup may
// recycle: no symlinks, no junctions/reparse points, nothing younger than
// minAge, and optionally only names matching the pattern.
func eligibleJunkChildren(dir string, minAge time.Duration, match func(string) bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-minAge)
	var out []string
	for _, e := range entries {
		if len(out) >= junkMaxChildrenPerItem {
			break
		}
		if e.Type()&os.ModeSymlink != 0 {
			continue
		}
		full := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		if isReparsePoint(full, info) {
			continue
		}
		if minAge > 0 && info.ModTime().After(cutoff) {
			continue
		}
		if match != nil && !match(e.Name()) {
			continue
		}
		out = append(out, full)
	}
	return out
}

// junkSources enumerates every candidate cleanup location on the machine.
func junkSources() []JunkItem {
	lad := localAppData()
	rad := roamingAppData()
	sys := systemRoot()
	var items []JunkItem

	add := func(cat JunkCategory, title, path, desc, warning string, admin, contentsOnly bool, minAge time.Duration, match func(string) bool) {
		if path == "" || junkPathIsSensitive(path) {
			return
		}
		items = append(items, JunkItem{
			Category: cat, CategoryTitle: junkCategoryTitles[cat], Title: title,
			Path: path, Description: desc, RequiresAdmin: admin,
			Warning: warning, contentsOnly: contentsOnly, minAge: minAge,
			matchPattern: match,
		})
	}

	// --- Temporary files ---
	if t := os.Getenv("TEMP"); t != "" {
		add(JunkCatTemp, "User temporary files", t,
			"Installer leftovers, crash artifacts, and scratch files older than 24 hours. Recently written files are kept: running apps and installs may still be using them.",
			"", false, true, tempMinAge, nil)
	}
	add(JunkCatTemp, "Windows temporary files", filepath.Join(sys, "Temp"),
		"System-level temp files older than 24 hours, including those from services and installers running as SYSTEM.",
		"", true, true, tempMinAge, nil)

	// --- Browser caches (only true cache directories; profiles enumerated explicitly) ---
	type browserDef struct {
		userData string
		label    string
	}
	browsers := []browserDef{
		{filepath.Join(lad, `Google\Chrome\User Data`), "Chrome"},
		{filepath.Join(lad, `Microsoft\Edge\User Data`), "Edge"},
		{filepath.Join(lad, `BraveSoftware\Brave-Browser\User Data`), "Brave"},
	}
	cacheDirNames := []string{"Cache", "Code Cache", "GPUCache"}
	for _, b := range browsers {
		for _, spec := range junkProfileCacheDirs(b.userData, []string{"Default", "Profile *"}, cacheDirNames, b.label) {
			add(JunkCatBrowser, spec.title, spec.path,
				"Downloaded images, scripts, and compiled code that browsers re-fetch automatically. Sign-ins, passwords, cookies, extensions, and history are never touched.",
				"Close the browser first for a complete cleanup; open files are skipped automatically.",
				false, true, 0, nil)
		}
	}
	// Firefox keeps its cache outside the Chromium "User Data" layout.
	if ffProfiles := junkCacheDir(filepath.Join(lad, `Mozilla\Firefox\Profiles`)); ffProfiles != "" {
		if entries, err := os.ReadDir(ffProfiles); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				for _, cn := range []string{"cache2", "startupCache", "shader-cache"} {
					p := filepath.Join(ffProfiles, e.Name(), cn)
					if junkCacheDir(p) == "" {
						continue
					}
					add(JunkCatBrowser, fmt.Sprintf("Firefox cache (%s, %s)", e.Name(), cn), p,
						"Firefox HTTP cache, startup cache, and shader cache. Cookies, logins, and history live in the profile root and are never touched.",
						"Close Firefox first for a complete cleanup; open files are skipped automatically.",
						false, true, 0, nil)
				}
			}
		}
	}
	// Classic Teams client cache.
	for _, cn := range []string{`Cache`, `GPUCache`, `Code Cache`} {
		if p := junkCacheDir(filepath.Join(rad, `Microsoft\Teams`, cn)); p != "" {
			add(JunkCatBrowser, "Teams cache", p,
				"Electron cache of the classic Teams client. Chats and credentials are not stored here.",
				"", false, true, 0, nil)
		}
	}

	// --- Windows Update leftovers ---
	add(JunkCatUpdate, "Windows Update download cache", filepath.Join(sys, `SoftwareDistribution\Download`),
		"Completed update payloads that already installed. Windows re-downloads anything it ever needs again.",
		"Updates still downloading are kept (files younger than 24h).", true, true, tempMinAge, nil)
	add(JunkCatUpdate, "Delivery Optimization cache", filepath.Join(sys, `ServiceProfiles\NetworkService\AppData\Local\Microsoft\Windows\DeliveryOptimization\Cache`),
		"Peer-to-peer update chunks shared with other PCs. Safe to clear; Windows re-fetches on demand.",
		"", true, true, tempMinAge, nil)

	// --- GPU shader caches ---
	shaderCaches := []struct {
		path  string
		title string
	}{
		{filepath.Join(lad, `D3DSCache`), "DirectX shader cache"},
		{filepath.Join(lad, `NVIDIA\DXCache`), "NVIDIA DirectX shader cache"},
		{filepath.Join(lad, `NVIDIA\GLCache`), "NVIDIA OpenGL shader cache"},
		{filepath.Join(lad, `AMD\DxCache`), "AMD DirectX shader cache"},
		{filepath.Join(lad, `AMD\DxcCache`), "AMD DXC shader cache"},
	}
	for _, sc := range shaderCaches {
		if junkCacheDir(sc.path) != "" {
			add(JunkCatShader, sc.title, sc.path,
				"Compiled GPU shader bytecode. It is rebuilt automatically the first time each game or app runs, at the cost of a few extra seconds of loading.",
				"", false, true, 0, nil)
		}
	}

	// --- Thumbnail & icon caches ---
	thumbDir := filepath.Join(lad, `Microsoft\Windows\Explorer`)
	if junkCacheDir(thumbDir) != "" {
		add(JunkCatThumbs, "Thumbnail & icon caches", thumbDir,
			"Explorer's cached thumbnail (thumbcache_*.db) and icon (iconcache_*.db) databases. They rebuild automatically as you browse folders.",
			"Files locked by Explorer are skipped; run again after restarting Explorer for a full clean.",
			false, true, 0, func(name string) bool {
				low := strings.ToLower(name)
				return strings.HasPrefix(low, "thumbcache_") || strings.HasPrefix(low, "iconcache_")
			})
	}

	// --- Error reports & crash dumps ---
	for _, p := range []string{
		filepath.Join(lad, `Microsoft\Windows\WER\ReportArchive`),
		filepath.Join(lad, `Microsoft\Windows\WER\ReportQueue`),
		filepath.Join(lad, `Microsoft\Windows\WER\Temp`),
	} {
		if junkCacheDir(p) != "" {
			add(JunkCatLogs, "Windows Error Reporting archives", p,
				"Queued and archived crash/debug reports sent (or waiting to be sent) to Microsoft. Nothing on this PC needs them.",
				"", false, true, logsMinAge, nil)
		}
	}
	add(JunkCatLogs, "Per-app crash dumps", filepath.Join(lad, "CrashDumps"),
		"Local .dmp crash snapshots written when applications crashed.",
		"", false, true, logsMinAge, func(name string) bool { return strings.HasSuffix(strings.ToLower(name), ".dmp") })
	add(JunkCatLogs, "Kernel minidumps", filepath.Join(sys, "Minidump"),
		"Small Blue Screen (BSOD) crash dumps collected by Windows.",
		"", true, true, logsMinAge, func(name string) bool { return strings.HasSuffix(strings.ToLower(name), ".dmp") })
	if _, err := os.Stat(filepath.Join(sys, "MEMORY.DMP")); err == nil {
		add(JunkCatLogs, "Full kernel memory dump", filepath.Join(sys, "MEMORY.DMP"),
			"Complete RAM snapshot written at the moment of a Blue Screen crash. Usually hundreds of MB to several GB.",
			"", true, false, 0, nil)
	}
	if junkCacheDir(filepath.Join(sys, "LiveKernelReports")) != "" {
		add(JunkCatLogs, "Live kernel reports", filepath.Join(sys, "LiveKernelReports"),
			"Dumps written when a component (usually graphics) recovered without a full crash.",
			"", true, true, logsMinAge, func(name string) bool { return strings.HasSuffix(strings.ToLower(name), ".dmp") })
	}

	// --- Previous Windows installation ---
	if _, err := os.Stat(filepath.Join(filepath.Dir(sys), "Windows.old")); err == nil {
		add(JunkCatWindowsOld, "Previous Windows installation (Windows.old)", filepath.Join(filepath.Dir(sys), "Windows.old"),
			"The complete previous Windows installation kept after an upgrade — often 15–40 GB. Windows deletes it automatically after 10 days.",
			"Removing this ends your ability to roll back to the previous Windows version.",
			true, true, 0, nil)
	}

	// --- Orphaned uninstall registry entries ---
	items = append(items, orphanUninstallEntries()...)

	return items
}

var guidBracketRe = regexp.MustCompile(`\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}$`)

// orphanUninstallEntries finds Win32 uninstall registry keys whose program is
// provably gone (uninstaller executable AND install location both missing),
// excluding MSI/GUID keys (owned by Windows Installer) and system components.
// These "ghost" entries still clutter Windows' Apps list.
func orphanUninstallEntries() []JunkItem {
	var out []JunkItem
	apps, _, err := ScanInstalledApps()
	if err != nil {
		return nil
	}
	for _, app := range apps {
		if app.Source != "Win32" || app.IsSystemComponent || app.RegistryPath == "" {
			continue
		}
		if guidBracketRe.MatchString(app.RegistryPath) {
			continue // Never touch MSI product keys.
		}
		if isAppStillInstalled(app) {
			continue
		}
		if app.Name == "" {
			continue
		}
		if _, _, err := ValidateCleanRegistryPath(app.RegistryPath); err != nil {
			continue
		}
		out = append(out, JunkItem{
			Category:      JunkCatOrphanReg,
			CategoryTitle: junkCategoryTitles[JunkCatOrphanReg],
			Title:         "Ghost entry: " + app.Name,
			Path:          app.RegistryPath,
			Description: fmt.Sprintf("This uninstall entry remains even though its uninstaller (%s) and install location are gone. It still shows up in Windows' Apps list.",
				orElse(exePath(app.UninstallString), "program")),
			IsRegistry: true,
		})
	}
	return out
}

// ScanJunk audits all system junk locations. It changes nothing.
func ScanJunk() JunkReport {
	t0 := time.Now()
	elevated := isElevated()
	candidates := junkSources()

	// Measure candidates concurrently (I/O bound).
	results := make([]JunkItem, len(candidates))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range candidates {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem; _ = recover() }()
			c := candidates[i]
			bytes, count := measureJunkItem(c)
			c.SizeBytes, c.ItemCount = bytes, count
			c.SizeFormatted = FormatBytes(bytes)
			c.DeletionMode = junkDeletionModes[c.Category]
			results[i] = c
		}(i)
	}
	wg.Wait()

	// Query the Recycle Bin so its size reflects reality.
	rbSize, rbCount := queryRecycleBinSize()
	if rbCount > 0 {
		results = append(results, JunkItem{
			Category: JunkCatRecycleBin, CategoryTitle: junkCategoryTitles[JunkCatRecycleBin],
			Title: "Recycle Bin contents", Path: "RECYCLE_BIN",
			Description: fmt.Sprintf("%d deleted item(s) still occupying disk space. Emptying the Recycle Bin permanently erases them — including anything WinBGM's own cleanup moved there earlier.", rbCount),
			SizeBytes:   rbSize, SizeFormatted: FormatBytes(rbSize), ItemCount: rbCount,
			Warning: "This permanently deletes everything in the Recycle Bin, from all apps.",
		})
	}

	var out []JunkItem
	var total uint64
	newCache := make(map[string]JunkItem)
	for i, it := range results {
		if it.Path == "" {
			continue
		}
		if it.Category != JunkCatRecycleBin && it.Category != JunkCatOrphanReg && it.SizeBytes == 0 && it.ItemCount == 0 {
			continue // Nothing to clean here.
		}
		it.ID = fmt.Sprintf("junk_%d", i)
		it.IsSelected = false
		out = append(out, it)
		newCache[it.ID] = it
		total += it.SizeBytes
	}

	sort.SliceStable(out, func(a, b int) bool {
		ca, cb := junkCategoryOrder[out[a].Category], junkCategoryOrder[out[b].Category]
		if ca != cb {
			return ca < cb
		}
		return out[a].SizeBytes > out[b].SizeBytes
	})

	junkCacheMu.Lock()
	junkCache = newCache
	junkCacheMu.Unlock()

	return JunkReport{
		Items:              out,
		TotalSizeBytes:     total,
		TotalSizeFormatted: FormatBytes(total),
		ScannableCount:     len(out),
		ScannedAt:          time.Now().Format(time.RFC3339),
		DurationMs:         time.Since(t0).Milliseconds(),
		Elevated:           elevated,
	}
}

// ============================================================================
// Cleaning
// ============================================================================

// JunkProgress is emitted while CleanJunkItems works so the UI can show live
// progress instead of a frozen-looking dialog on large folders.
type JunkProgress struct {
	Phase        string `json:"phase"`                  // "start" | "item" | "done"
	Index        int    `json:"index"`                  // 1-based index of the current item
	Total        int    `json:"total"`                  // number of selected items
	Title        string `json:"title,omitempty"`        // display name of the current item
	Mode         string `json:"mode,omitempty"`         // "permanent" | "recycle" | "registry" | "recyclebin"
	FilesDeleted int    `json:"filesDeleted,omitempty"` // intra-item counter (permanent mode)
}

var (
	junkProgressMu sync.Mutex
	junkProgressFn func(JunkProgress)
)

// SetJunkProgressFunc installs (or, with nil, removes) the callback invoked
// during CleanJunkItems. It exists so the Wails app layer can bridge progress
// to the frontend without the cleaner depending on Wails.
func SetJunkProgressFunc(fn func(JunkProgress)) {
	junkProgressMu.Lock()
	junkProgressFn = fn
	junkProgressMu.Unlock()
}

func emitJunkProgress(p JunkProgress) {
	junkProgressMu.Lock()
	fn := junkProgressFn
	junkProgressMu.Unlock()
	if fn != nil {
		fn(p)
	}
}

// CleanJunkItems cleans the selected junk items. Every request is verified
// against the scan cache first: an item whose ID is unknown or whose path no
// longer matches is rejected, exactly like the residual cleaner.
func CleanJunkItems(items []JunkItem) (CleanResult, error) {
	var result CleanResult
	elevated := isElevated()
	usedRecycleBin := false

	total := 0
	for _, reqItem := range items {
		if reqItem.IsSelected {
			total++
		}
	}
	emitJunkProgress(JunkProgress{Phase: "start", Total: total})

	idx := 0
	for _, reqItem := range items {
		if !reqItem.IsSelected {
			continue
		}
		idx++
		junkCacheMu.Lock()
		authItem, ok := junkCache[reqItem.ID]
		junkCacheMu.Unlock()
		if !ok || !strings.EqualFold(authItem.Path, reqItem.Path) {
			result.Errors = append(result.Errors, fmt.Sprintf("Rejected unverified cleanup item %q: please scan again", reqItem.ID))
			continue
		}
		if authItem.RequiresAdmin && !elevated {
			result.Errors = append(result.Errors, fmt.Sprintf("Skipped %q: administrator rights are required (use Restart as administrator)", authItem.Title))
			continue
		}

		mode := "permanent"
		switch {
		case authItem.Category == JunkCatRecycleBin:
			mode = "recyclebin"
		case authItem.IsRegistry:
			mode = "registry"
		case reqItem.UseRecycleBin:
			mode = "recycle"
		}
		emitJunkProgress(JunkProgress{Phase: "item", Index: idx, Total: total, Title: authItem.Title, Mode: mode})

		switch {
		case authItem.Category == JunkCatRecycleBin:
			if err := emptyRecycleBin(); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Recycle Bin: %v", err))
				continue
			}
			result.FreedBytes += authItem.SizeBytes
			result.DeletedFilesCount += authItem.ItemCount

		case authItem.IsRegistry:
			if _, err := exportRegistryBackup(authItem.Path); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Aborted %q: backup failed: %v", authItem.Title, err))
				continue
			}
			if err := SafeDeleteRegistryKey(authItem.Path); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Registry error on %s: %v", authItem.Path, err))
				continue
			}
			result.DeletedKeysCount++

		default:
			var targets []string
			if authItem.contentsOnly {
				targets = eligibleJunkChildren(authItem.Path, authItem.minAge, authItem.matchPattern)
			} else if _, err := os.Lstat(authItem.Path); err == nil {
				targets = []string{authItem.Path}
			}
			if len(targets) == 0 {
				continue
			}
			// Policy: permanent deletion (the only mode that actually frees
			// space, and safe because every junk category is regenerable) —
			// unless the UI explicitly asked for the recoverable Recycle Bin
			// move. The UI can only downgrade, never upgrade, the policy.
			var freed uint64
			var files int
			var errs []string
			if reqItem.UseRecycleBin {
				freed, files, errs = recycleJunkPaths(targets)
				if files > 0 {
					usedRecycleBin = true
				}
			} else {
				// Throttled per-file progress: one Wails event per file would
				// flood the bridge on folders with thousands of entries.
				var nDeleted int
				lastEmit := time.Now()
				onFile := func(n int) {
					nDeleted += n
					if now := time.Now(); now.Sub(lastEmit) >= 60*time.Millisecond {
						lastEmit = now
						emitJunkProgress(JunkProgress{Phase: "item", Index: idx, Total: total, Title: authItem.Title, Mode: mode, FilesDeleted: nDeleted})
					}
				}
				freed, files, errs = permanentlyDeleteJunkTargets(targets, onFile)
				emitJunkProgress(JunkProgress{Phase: "item", Index: idx, Total: total, Title: authItem.Title, Mode: mode, FilesDeleted: nDeleted})
			}
			result.FreedBytes += freed
			result.DeletedFilesCount += files
			result.Errors = append(result.Errors, errs...)
		}
	}
	emitJunkProgress(JunkProgress{Phase: "done", Index: total, Total: total})

	result.FreedSizeFormatted = FormatBytes(result.FreedBytes)

	if result.DeletedFilesCount > 0 || result.DeletedKeysCount > 0 || len(result.Errors) > 0 {
		var summary string
		if usedRecycleBin {
			summary = fmt.Sprintf("Freed %s (%d files, %d registry keys) — items were moved to the Recycle Bin and can be restored from there.", result.FreedSizeFormatted, result.DeletedFilesCount, result.DeletedKeysCount)
		} else {
			summary = fmt.Sprintf("Freed %s (%d files, %d registry keys) — regenerable junk deleted permanently; the Recycle Bin was not used.", result.FreedSizeFormatted, result.DeletedFilesCount, result.DeletedKeysCount)
		}
		if len(result.Errors) > 0 {
			summary += fmt.Sprintf(" %d item(s) reported errors.", len(result.Errors))
		}
		_ = appendHistory(HistoryEntry{
			ID:      fmt.Sprintf("junkclean-%d", time.Now().UnixNano()),
			Time:    time.Now().Format(time.RFC3339),
			Title:   "Junk & cache cleanup",
			Summary: summary,
			Status:  "applied",
			// Permanently deleted junk cannot be restored by design; it is
			// regenerable by definition (caches, temp files, dumps).
			Restorable: false,
		})
	}

	return result, nil
}

// permanentlyDeleteJunkTargets removes regenerable junk targets for real —
// the only mode that frees disk space immediately (a Recycle Bin move on the
// same volume just renames the files into $Recycle.Bin, occupying the space
// until the bin is emptied). Every junk category this is used for holds
// artifacts Windows or the owning app rebuilds automatically. Guards: paths
// come from the scan cache, reparse points are never followed, sensitive
// basenames are re-checked, and long paths get the \\?\ prefix.
func permanentlyDeleteJunkTargets(targets []string, onFile func(deleted int)) (freed uint64, files int, errs []string) {
	for _, p := range targets {
		fi, err := os.Lstat(p)
		if err != nil {
			continue // vanished between scan and clean: nothing to do
		}
		if isReparsePoint(p, fi) {
			errs = append(errs, fmt.Sprintf("%s: skipped (symlink or junction)", p))
			continue
		}
		if junkPathIsSensitive(p) {
			errs = append(errs, fmt.Sprintf("%s: refused (sensitive path)", p))
			continue
		}
		var size uint64
		if fi.IsDir() {
			size, _ = ComputeFolderSize(p, junkMaxChildrenPerItem)
		} else {
			size = uint64(fi.Size())
		}
		n, err := deleteTreeProgress(p, onFile)
		files += n
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p, err))
			if n == 0 {
				continue
			}
		}
		freed += size
	}
	return freed, files, errs
}

// deleteTreeProgress deletes p, reporting every file removed through onFile so
// the UI can show live progress inside large folders. It deletes children
// one-by-one (deepest-first via recursion) instead of one RemoveAll call,
// which turns a minutes-long silent stall on huge caches (Delivery
// Optimization, shader caches) into visible, per-file progress — and lets
// partial failures remove everything that could be removed. Falls back to a
// plain RemoveAll when the directory cannot be enumerated.
func deleteTreeProgress(p string, onFile func(deleted int)) (int, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return 0, nil // vanished: nothing to do
	}
	if isReparsePoint(p, fi) {
		return 0, fmt.Errorf("skipped (symlink or junction)")
	}
	lp := longPathSafe(p)
	if !fi.IsDir() {
		if err := os.Remove(lp); err != nil {
			return 0, err
		}
		if onFile != nil {
			onFile(1)
		}
		return 1, nil
	}
	entries, err := os.ReadDir(lp)
	if err != nil {
		// Cannot enumerate (locked or access denied): try the whole tree.
		if err := os.RemoveAll(lp); err != nil {
			return 0, err
		}
		return 0, nil
	}
	deleted := 0
	var firstErr error
	for _, e := range entries {
		n, err := deleteTreeProgress(filepath.Join(p, e.Name()), onFile)
		deleted += n
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := os.Remove(lp); err != nil && firstErr == nil {
		firstErr = err
	}
	return deleted, firstErr
}

// longPathSafe prefixes paths longer than the classic MAXPATH limit with the
// Windows extended-length prefix so deep node_modules-style junk still deletes.
func longPathSafe(p string) string {
	if len(p) >= 248 && !strings.HasPrefix(p, `\\?\`) && filepath.IsAbs(p) {
		return `\\?\` + p
	}
	return p
}

// recycleJunkPaths moves paths to the Recycle Bin in batches of 64 (one
// SHFileOperationW call per batch with a double-null-terminated path list).
// If a batch fails, it retries the batch file-by-file so one locked file
// doesn't discard the rest, and reports every failure individually.
func recycleJunkPaths(paths []string) (freed uint64, files int, errs []string) {
	sizeOf := func(p string) uint64 {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return uint64(fi.Size())
		}
		return 0
	}

	const chunk = 64
	for start := 0; start < len(paths); start += chunk {
		end := start + chunk
		if end > len(paths) {
			end = len(paths)
		}
		batch := paths[start:end]
		if err := shellRecycleBatch(batch); err == nil {
			for _, p := range batch {
				freed += sizeOf(p)
			}
			files += len(batch)
			continue
		}
		// Batch failed: retry individually to salvage what we can.
		for _, p := range batch {
			if err := shellRecycleBatch([]string{p}); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", p, err))
				continue
			}
			freed += sizeOf(p)
			files++
		}
	}
	return freed, files, errs
}

// shellRecycleBatch calls SHFileOperationW with FOF_ALLOWUNDO so the shell
// moves the paths to the Recycle Bin. All-UI-off flags mean a full Recycle
// Bin makes the operation fail (reported as an error) rather than silently
// permanent-deleting: WinBGM never destroys data the user asked to keep.
//
// The pFrom list is built as raw UTF-16 with NUL separators and a trailing
// double NUL — windows.UTF16FromString / UTF16PtrFromString cannot be used
// for the joined list because they reject embedded NULs (EINVAL "invalid
// argument"), which silently disabled the whole recycle path once.
func shellRecycleBatch(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	list, err := buildUTF16PathList(paths)
	if err != nil {
		return err
	}
	op := shFileOpStructW{
		wFunc:  foDelete,
		pFrom:  &list[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI,
	}
	ret, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if op.fAnyOperationsAborted != 0 {
		return fmt.Errorf("operation aborted by Windows (Recycle Bin may be disabled or full)")
	}
	if ret != 0 {
		return fmt.Errorf("SHFileOperationW failed with code 0x%X", ret)
	}
	return nil
}

// buildUTF16PathList encodes each path as NUL-terminated UTF-16 and appends a
// final NUL, producing the double-null-terminated multi-path string that
// SHFILEOPSTRUCTW.pFrom expects.
func buildUTF16PathList(paths []string) ([]uint16, error) {
	var list []uint16
	for _, p := range paths {
		if p == "" {
			return nil, fmt.Errorf("empty path in cleanup list")
		}
		enc, err := windows.UTF16FromString(filepath.Clean(p))
		if err != nil {
			return nil, err // real paths can't contain NULs; this is a bug guard
		}
		list = append(list, enc...) // UTF16FromString already NUL-terminates
	}
	list = append(list, 0) // final double NUL terminator
	return list, nil
}

// ============================================================================
// Recycle Bin query & empty
// ============================================================================

// shQueryRBInfo mirrors SHQUERYRBINFO with explicit padding so the layout is
// identical on 386, amd64, and arm64 (Go aligns int64 to 4 bytes on 386,
// Windows expects 8).
type shQueryRBInfo struct {
	CBSize   uint32
	_pad0    uint32
	Size     uint64
	NumItems uint32
	_pad1    uint32
}

var (
	procSHQueryRecycleBinW = modShell32Recycle.NewProc("SHQueryRecycleBinW")
	procSHEmptyRecycleBinW = modShell32Recycle.NewProc("SHEmptyRecycleBinW")
)

const (
	sherbNoConfirmation = 0x0001
	sherbNoProgressUI   = 0x0002
	sherbNoSound        = 0x0004
)

func queryRecycleBinSize() (uint64, int) {
	var total uint64
	var items int
	for c := 'C'; c <= 'Z'; c++ {
		root := string(c) + `:\`
		if _, err := os.Stat(root); err != nil {
			continue
		}
		rootPtr, err := windows.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		var q shQueryRBInfo
		q.CBSize = uint32(unsafe.Sizeof(q))
		ret, _, _ := procSHQueryRecycleBinW.Call(
			uintptr(unsafe.Pointer(rootPtr)),
			uintptr(unsafe.Pointer(&q)),
		)
		if ret != 0 {
			continue // No bin on this drive or access denied.
		}
		total += q.Size
		items += int(q.NumItems)
	}
	return total, items
}

func emptyRecycleBin() error {
	flags := uintptr(sherbNoConfirmation | sherbNoProgressUI | sherbNoSound)
	ret, _, _ := procSHEmptyRecycleBinW.Call(0, 0, flags)
	if ret != 0 {
		return fmt.Errorf("SHEmptyRecycleBinW failed with HRESULT 0x%X", ret)
	}
	return nil
}
