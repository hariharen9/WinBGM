package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type nmLocation struct {
	root  registry.Key
	label string
	path  string
}

var nmLocations = []nmLocation{
	{registry.CURRENT_USER, "Chrome Native Messaging (User)", `Software\Google\Chrome\NativeMessagingHosts`},
	{registry.LOCAL_MACHINE, "Chrome Native Messaging (Machine)", `SOFTWARE\Google\Chrome\NativeMessagingHosts`},
	{registry.CURRENT_USER, "Edge Native Messaging (User)", `Software\Microsoft\Edge\NativeMessagingHosts`},
	{registry.LOCAL_MACHINE, "Edge Native Messaging (Machine)", `SOFTWARE\Microsoft\Edge\NativeMessagingHosts`},
	{registry.CURRENT_USER, "Firefox Native Messaging (User)", `Software\Mozilla\NativeMessagingHosts`},
	{registry.LOCAL_MACHINE, "Firefox Native Messaging (Machine)", `SOFTWARE\Mozilla\NativeMessagingHosts`},
}

type nmManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

func scanNativeMessaging(pt procTable, _ bool) ([]Component, error) {
	var out []Component
	seen := map[string]bool{}

	for _, loc := range nmLocations {
		k, err := registry.OpenKey(loc.root, loc.path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		hostNames, _ := k.ReadSubKeyNames(0)
		for _, name := range hostNames {
			sk, err := registry.OpenKey(k, name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			manifestPath, _, _ := sk.GetStringValue("")
			sk.Close()

			if manifestPath == "" {
				continue
			}

			dedup := strings.ToLower(loc.label + "|" + name)
			if seen[dedup] {
				continue
			}
			seen[dedup] = true

			manifestPath = os.ExpandEnv(manifestPath)
			manifestBytes, err := os.ReadFile(manifestPath)
			if err != nil {
				continue
			}

			var mf nmManifest
			if err := json.Unmarshal(manifestBytes, &mf); err != nil {
				continue
			}

			binPath := mf.Path
			if !filepath.IsAbs(binPath) {
				binPath = filepath.Clean(filepath.Join(filepath.Dir(manifestPath), binPath))
			}
			binPath = os.ExpandEnv(binPath)

			pids := pt.pidsFor(binPath)
			desc := mf.Description
			if desc == "" {
				desc = mf.Name
			}

			out = append(out, Component{
				ID:        "nativemessaging:" + rootName(loc.root) + ":" + name,
				Kind:      "nativemessaging",
				Name:      name,
				Display:   desc,
				Command:   binPath,
				ExePath:   binPath,
				Source:    loc.label,
				Trigger:   "When browser extension launches host",
				StartType: "Browser Stdio Bridge",
				Enabled:   true,
				AutoStart: false,
				Running:   len(pids) > 0,
				PIDs:      pids,
				tgt: Target{
					Kind: "nativemessaging",
					Root: rootName(loc.root),
					Path: loc.path,
					Name: name,
				},
			})
		}
		k.Close()
	}

	return out, nil
}
