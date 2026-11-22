package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

var kindOrder = map[string]int{
	"service": 0, "run": 1, "folder": 2, "task": 3,
	"process": 4, "storeapp": 5, "winlogon": 6, "wmi": 7,
}

func buildInventory() Inventory {
	t0 := time.Now()
	pt := snapshotProcs()
	elevated := isElevated()
	var warns []string

	type scanner struct {
		id string
		fn func(procTable, bool) ([]Component, error)
	}
	scanners := []scanner{
		{"service", scanServices},
		{"run", scanRun},
		{"folder", scanFolders},
		{"task", scanTasks},
		{"storeapp", scanStoreApps},
		{"winlogon", scanWinlogon},
		{"wmi", scanWMI},
	}
	results := make([][]Component, len(scanners))
	errs := make([]error, len(scanners))
	var wg sync.WaitGroup
	for i, s := range scanners {
		wg.Add(1)
		go func(i int, s scanner) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errs[i] = fmt.Errorf("scanner crashed: %v", r)
				}
			}()
			results[i], errs[i] = s.fn(pt, elevated)
		}(i, s)
	}
	wg.Wait()

	counts := map[string]int{}
	var comps []Component
	for i, s := range scanners {
		counts[s.id] = len(results[i])
		comps = append(comps, results[i]...)
		if errs[i] != nil {
			warns = append(warns, s.id+": "+errs[i].Error())
		}
	}
	for i := range comps {
		c := &comps[i]
		if c.ExePath == "" && c.Command != "" && c.Kind != "folder" {
			c.ExePath = exePath(c.Command)
		}
	}

	// Processes that run without any visible window and are not explained by another entry.
	procComps := backgroundProcesses(pt, comps)
	counts["process"] = len(procComps)
	comps = append(comps, procComps...)

	enrich(comps, pt)
	apps := group(comps, pt)
	for i := range apps {
		for j := range apps[i].Components {
			computeControl(&apps[i].Components[j], elevated)
		}
	}

	var totalRAM uint64
	seenPIDs := map[uint32]bool{}
	for _, app := range apps {
		for _, c := range app.Components {
			for _, pid := range c.PIDs {
				if !seenPIDs[pid] {
					seenPIDs[pid] = true
					if p, ok := pt.byPID[pid]; ok {
						totalRAM += p.RAM
					}
				}
			}
		}
	}

	if !elevated {
		warns = append(warns, "Not running as administrator: some processes can't be inspected and service lock status is not checked.")
	}
	return Inventory{
		Apps:              apps,
		Sources:           sourceList(counts),
		Warnings:          warns,
		ScannedAt:         time.Now().Format(time.RFC3339),
		DurationMs:        time.Since(t0).Milliseconds(),
		Machine:           hostName(),
		OS:                osName(),
		Elevated:          elevated,
		AlwaysAdmin:       checkAlwaysAdmin(),
		TotalRAMBytes:     totalRAM,
		TotalRAMFormatted: formatRAM(totalRAM),
	}
}

func backgroundProcesses(pt procTable, existing []Component) []Component {
	self := uint32(os.Getpid())
	skip := pt.descendantsOf(self)
	covered := map[string]bool{}
	for _, c := range existing {
		if c.ExePath != "" {
			covered[strings.ToLower(c.ExePath)] = true
		}
	}
	type agg struct {
		pids   []uint32
		window bool
		ppid   uint32
		path   string
	}
	byPath := map[string]*agg{}
	for _, p := range pt.list {
		if p.Path == "" || skip[p.PID] {
			continue
		}
		if _, _, sys := ownerOf(p.Path); sys {
			continue
		}
		k := strings.ToLower(p.Path)
		a := byPath[k]
		if a == nil {
			a = &agg{path: p.Path, ppid: p.PPID}
			byPath[k] = a
		}
		a.pids = append(a.pids, p.PID)
		a.window = a.window || p.Window
	}
	var out []Component
	for k, a := range byPath {
		if a.window || covered[k] {
			continue
		}
		trig := "Started by a process that has since exited"
		if par, ok := pt.byPID[a.ppid]; ok {
			if strings.EqualFold(par.Name, "svchost.exe") {
				trig = "Started by Windows (background task or notification)"
			} else {
				trig = "Started by " + par.Name
			}
		}
		out = append(out, Component{
			Kind: "process", Name: filepath.Base(a.path), Command: a.path, ExePath: a.path,
			Source: "Running processes", Trigger: trig, Enabled: true, Running: true, PIDs: a.pids,
		})
	}
	return out
}

func enrich(comps []Component, pt procTable) {
	infos := map[string]exeInfo{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, c := range comps {
		if c.ExePath == "" {
			continue
		}
		mu.Lock()
		_, seen := infos[c.ExePath]
		if !seen {
			infos[c.ExePath] = exeInfo{}
		}
		mu.Unlock()
		if seen {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(p string) {
			defer wg.Done()
			defer func() { <-sem; _ = recover() }()
			info := lookupInfo(p)
			mu.Lock()
			infos[p] = info
			mu.Unlock()
		}(c.ExePath)
	}
	wg.Wait()

	for i := range comps {
		c := &comps[i]
		c.ID = strings.ToLower(c.Kind + "|" + c.Source + "|" + c.Name)
		c.Signature = "n/a"

		if len(c.PIDs) > 0 {
			c.RAMBytes = pt.ramFor(c.PIDs)
			c.RAMFormatted = formatRAM(c.RAMBytes)
		}

		if c.ExePath == "" {
			continue
		}
		info := infos[c.ExePath]
		c.Publisher, c.Product = info.Company, info.Product
		c.SHA256 = info.SHA256
		if c.SHA256 != "" {
			c.VirusTotalURL = "https://www.virustotal.com/gui/file/" + c.SHA256
		}

		switch {
		case !info.Exists:
			c.Signature = "missing"
		case info.Sig == "valid":
			c.Signature = "valid"
		default:
			c.Signature = "unsigned"
		}
		low := strings.ToLower(c.ExePath)
		if !info.Exists && c.Enabled && c.Kind != "process" && c.Kind != "storeapp" {
			c.Flags = append(c.Flags, "missing")
		}
		if strings.Contains(low, `\temp\`) || strings.Contains(low, `\downloads\`) {
			c.Flags = append(c.Flags, "temp")
		}
		if c.Signature == "unsigned" && strings.Contains(low, `\users\`) {
			c.Flags = append(c.Flags, "unsigned")
		}
		if isSuspiciousSystemName(c.ExePath) {
			c.Flags = append(c.Flags, "impersonation")
			c.Review = true
		}
		if len(c.Flags) > 0 {
			c.Review = true
		}
	}
}

func ownerFor(c *Component) (key, label string, sys bool) {
	if c.ownerKey != "" {
		return c.ownerKey, c.ownerLabel, c.ownerSystem
	}
	if c.ExePath != "" {
		if k, l, s := ownerOf(c.ExePath); k != "" {
			return k, l, s
		}
	}
	n := strings.TrimSuffix(c.Name, filepath.Ext(c.Name))
	return "name:" + strings.ToLower(n), n, false
}

func group(comps []Component, pt procTable) []AppGroup {
	groups := map[string]*AppGroup{}
	prods := map[string]map[string]int{}
	comps2 := map[string]map[string]int{}
	var keys []string
	for i := range comps {
		c := comps[i]
		key, label, sys := ownerFor(&c)
		c.System = sys
		g := groups[key]
		if g == nil {
			g = &AppGroup{ID: key, Name: label, System: true}
			groups[key] = g
			prods[key], comps2[key] = map[string]int{}, map[string]int{}
			keys = append(keys, key)
		}
		g.Components = append(g.Components, c)
		g.System = g.System && sys
		if c.Product != "" {
			prods[key][c.Product]++
		}
		if c.Publisher != "" {
			comps2[key][c.Publisher]++
		}
	}

	out := make([]AppGroup, 0, len(keys))
	for _, key := range keys {
		g := groups[key]
		sort.SliceStable(g.Components, func(i, j int) bool {
			a, b := g.Components[i], g.Components[j]
			if kindOrder[a.Kind] != kindOrder[b.Kind] {
				return kindOrder[a.Kind] < kindOrder[b.Kind]
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		})
		// One shared product name beats a folder name.
		if len(prods[key]) == 1 {
			for p := range prods[key] {
				g.Name = p
			}
		}
		g.Publisher = top(comps2[key])
		g.Letter = "?"
		for _, r := range g.Name {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				g.Letter = strings.ToUpper(string(r))
				break
			}
		}

		var signIn, svcAuto, scheduled, procs int
		anyUnsigned, anyValid := false, false
		groupPIDs := map[uint32]bool{}

		for _, c := range g.Components {
			g.Running = g.Running || c.Running
			g.Review = g.Review || c.Review
			for _, pid := range c.PIDs {
				groupPIDs[pid] = true
			}
			if c.AutoStart {
				g.AutoStart = true
				if c.Kind == "service" {
					svcAuto++
				} else {
					signIn++
				}
			}
			if c.Kind == "task" && c.Enabled && !c.AutoStart {
				scheduled++
			}
			if c.Kind == "process" {
				procs++
			}
			switch c.Signature {
			case "valid":
				anyValid = true
			case "unsigned", "missing":
				anyUnsigned = true
			}
			if g.Path == "" && c.ExePath != "" && filepath.IsAbs(c.ExePath) && c.Signature != "missing" {
				g.Path = filepath.Dir(c.ExePath)
			}
		}

		var appRAM uint64
		for pid := range groupPIDs {
			if p, ok := pt.byPID[pid]; ok {
				appRAM += p.RAM
			}
		}
		g.RAMBytes = appRAM
		g.RAMFormatted = formatRAM(appRAM)

		switch {
		case anyUnsigned:
			g.Signature = "unverified"
		case anyValid:
			g.Signature = "verified"
		default:
			g.Signature = "unknown"
		}
		switch {
		case signIn > 0 && svcAuto > 0:
			g.Behavior, g.BehaviorNote = "At sign-in and boot", fmt.Sprintf("%d automatic entries", signIn+svcAuto)
		case signIn > 0:
			g.Behavior, g.BehaviorNote = "At sign-in", plural(signIn, "enabled entry", "enabled entries")
		case svcAuto > 0:
			g.Behavior, g.BehaviorNote = "Automatic service", plural(svcAuto, "automatic service", "automatic services")
		case scheduled > 0:
			g.Behavior, g.BehaviorNote = "On a schedule", plural(scheduled, "scheduled task", "scheduled tasks")
		case procs > 0:
			g.Behavior, g.BehaviorNote = "Started by Windows or another app", "No sign-in entry found"
		default:
			g.Behavior, g.BehaviorNote = "On demand", "No automatic launch"
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func top(m map[string]int) string {
	best, n := "", 0
	for k, v := range m {
		if v > n || (v == n && k < best) {
			best, n = k, v
		}
	}
	return best
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func sourceList(c map[string]int) []SourceInfo {
	return []SourceInfo{
		{"run", "Registry Run & RunOnce keys", "HKCU and HKLM Run/RunOnce entries, with Task Manager's enabled/disabled state.", c["run"], "scanned"},
		{"folder", "Startup folders", "Per-user and all-users Startup folders, with shortcut targets resolved.", c["folder"], "scanned"},
		{"service", "Windows services", "Every service with its startup type, running state and reconfigurability.", c["service"], "scanned"},
		{"task", "Task Scheduler", "Tasks outside Microsoft's own library, with triggers and actions.", c["task"], "scanned"},
		{"winlogon", "Winlogon & logon shell", "Logon shell overrides, Userinit handlers, and logon startup hooks.", c["winlogon"], "scanned"},
		{"wmi", "WMI event subscriptions", "Persistent WMI event filters and consumers that launch actions.", c["wmi"], "scanned"},
		{"storeapp", "Store app permissions", "Per-app background permission records for packaged apps.", c["storeapp"], "partial"},
		{"process", "Running processes without a window", "Programs running now with no visible window that no entry above explains.", c["process"], "partial"},
	}
}
