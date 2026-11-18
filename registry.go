package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const approvedBase = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\`

type runLoc struct {
	root     registry.Key
	label    string
	path     string
	approved string
}

var runLocs = []runLoc{
	{registry.CURRENT_USER, "HKCU Run", `Software\Microsoft\Windows\CurrentVersion\Run`, approvedBase + "Run"},
	{registry.LOCAL_MACHINE, "HKLM Run", `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, approvedBase + "Run"},
	{registry.LOCAL_MACHINE, "HKLM Run (32-bit)", `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`, approvedBase + "Run32"},
}

func scanRun(pt procTable, _ bool) ([]Component, error) {
	var out []Component
	for _, l := range runLocs {
		k, err := registry.OpenKey(l.root, l.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		names, _ := k.ReadValueNames(0)
		for _, n := range names {
			cmd, _, err := k.GetStringValue(n)
			if err != nil {
				continue
			}
			on := approved(l.root, l.approved, n)
			exe := exePath(cmd)
			pids := pt.pidsFor(exe)
			out = append(out, Component{
				Kind: "run", Name: n, Command: cmd, ExePath: exe, Source: l.label,
				Trigger: "At sign-in", Enabled: on, AutoStart: on,
				Running: len(pids) > 0, PIDs: pids,
				tgt: Target{Kind: "run", Root: rootName(l.root), Path: l.approved, Name: n},
			})
		}
		k.Close()
	}
	return out, nil
}

type lnkInfo struct{ File, Target, Args string }

func scanFolders(pt procTable, _ bool) ([]Component, error) {
	dirs := []struct {
		root  registry.Key
		label string
		dir   string
	}{
		{registry.CURRENT_USER, "Startup folder (user)", filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Startup`)},
		{registry.LOCAL_MACHINE, "Startup folder (all users)", filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs\StartUp`)},
	}

	targets := map[string]lnkInfo{}
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := fmt.Sprintf(`$s = New-Object -ComObject WScript.Shell
Get-ChildItem -LiteralPath %s,%s -Filter *.lnk -ErrorAction SilentlyContinue | ForEach-Object { $l = $s.CreateShortcut($_.FullName); [pscustomobject]@{ File = $_.FullName; Target = $l.TargetPath; Args = $l.Arguments } } | ConvertTo-Json -Compress`,
		q(dirs[0].dir), q(dirs[1].dir))
	if raw, err := ps(script); err == nil {
		if list, err := parseList[lnkInfo](raw); err == nil {
			for _, li := range list {
				targets[strings.ToLower(li.File)] = li
			}
		}
	}

	var out []Component
	for _, d := range dirs {
		files, err := os.ReadDir(d.dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			n := f.Name()
			if strings.EqualFold(n, "desktop.ini") {
				continue
			}
			full := filepath.Join(d.dir, n)
			on := approved(d.root, approvedBase+"StartupFolder", n)
			c := Component{Kind: "folder", Name: n, Command: full, Source: d.label,
				Trigger: "At sign-in", Enabled: on, AutoStart: on,
				tgt: Target{Kind: "folder", Root: rootName(d.root), Path: approvedBase + "StartupFolder", Name: n}}
			if li, ok := targets[strings.ToLower(full)]; ok && li.Target != "" {
				c.Command = strings.TrimSpace(li.Target + " " + li.Args)
				c.ExePath = li.Target
				c.PIDs = pt.pidsFor(li.Target)
				c.Running = len(c.PIDs) > 0
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// StartupApproved values: first byte even = enabled, odd = disabled (what Task Manager reads).
func approved(root registry.Key, path, name string) bool {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return true
	}
	defer k.Close()
	v, _, err := k.GetBinaryValue(name)
	if err != nil || len(v) == 0 {
		return true
	}
	return v[0]&1 == 0
}

// scanStoreApps lists per-app background permissions of Store/packaged apps (where Windows records them).
func scanStoreApps(pt procTable, _ bool) ([]Component, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\BackgroundAccessApplications`,
		registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return nil, nil
	}
	defer k.Close()
	subs, _ := k.ReadSubKeyNames(-1)
	var out []Component
	for _, fam := range subs {
		sk, err := registry.OpenKey(k, fam, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		dis, _, _ := sk.GetIntegerValue("Disabled")
		byUser, _, _ := sk.GetIntegerValue("DisabledByUser")
		sk.Close()

		name := strings.SplitN(fam, "_", 2)[0]
		allowed := dis == 0 && byUser == 0
		c := Component{
			Kind: "storeapp", Name: fam, Display: packageDisplay(name), Source: "Store app permissions",
			Trigger: "Windows may run it in the background", Enabled: allowed,
			ownerKey: "pkg:" + strings.ToLower(name), ownerLabel: packageDisplay(name),
			ownerSystem: isWindowsPackage(name),
		}
		needle := `\` + strings.ToLower(name) + `_`
		for p, pids := range pt.byPath {
			if strings.Contains(p, needle) {
				c.PIDs = append(c.PIDs, pids...)
			}
		}
		c.Running = len(c.PIDs) > 0
		out = append(out, c)
	}
	return out, nil
}
