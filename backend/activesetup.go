package backend

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

func scanActiveSetup(pt procTable, _ bool) ([]Component, error) {
	var out []Component

	roots := []struct {
		root  registry.Key
		label string
		path  string
	}{
		{registry.LOCAL_MACHINE, "Active Setup (64-bit)", `SOFTWARE\Microsoft\Active Setup\Installed Components`},
		{registry.LOCAL_MACHINE, "Active Setup (32-bit)", `SOFTWARE\WOW6432Node\Microsoft\Active Setup\Installed Components`},
	}

	seen := map[string]bool{}

	for _, r := range roots {
		k, err := registry.OpenKey(r.root, r.path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		subs, _ := k.ReadSubKeyNames(0)
		for _, name := range subs {
			sk, err := registry.OpenKey(k, name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			stubPath, _, _ := sk.GetStringValue("StubPath")
			dispName, _, _ := sk.GetStringValue("")
			sk.Close()

			if stubPath == "" {
				continue
			}

			key := strings.ToLower(name + "|" + stubPath)
			if seen[key] {
				continue
			}
			seen[key] = true

			exe := exePath(stubPath)
			pids := pt.pidsFor(exe)
			title := dispName
			if title == "" {
				title = name
			}

			out = append(out, Component{
				ID:        "activesetup:" + name,
				Kind:      "activesetup",
				Name:      name,
				Display:   title,
				Command:   stubPath,
				ExePath:   exe,
				Source:    r.label,
				Trigger:   "At first user sign-in before shell",
				StartType: "Active Setup Stub",
				Enabled:   true,
				AutoStart: true,
				Running:   len(pids) > 0,
				PIDs:      pids,
				tgt: Target{
					Kind: "activesetup",
					Root: "HKLM",
					Path: r.path + `\` + name,
					Name: "StubPath",
				},
			})
		}
		k.Close()
	}

	return out, nil
}
