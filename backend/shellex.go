package backend

import (
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

type shellExLocation struct {
	label string
	path  string
}

var shellExLocations = []shellExLocation{
	{"File Context Menu", `*\shellex\ContextMenuHandlers`},
	{"Folder Context Menu", `Directory\shellex\ContextMenuHandlers`},
	{"Desktop / Background Menu", `Directory\Background\shellex\ContextMenuHandlers`},
	{"Folder Shell Extensions", `Folder\shellex\ContextMenuHandlers`},
	{"Filesystem Context Menu", `AllFilesystemObjects\shellex\ContextMenuHandlers`},
}

// isCLSIDBlocked checks if the CLSID is registered in Windows Shell Extensions\Blocked.
func isCLSIDBlocked(clsid string) bool {
	if clsid == "" {
		return false
	}
	cleanCLSID := strings.Trim(strings.ToLower(clsid), "{}")

	checkKey := func(root registry.Key, path string) bool {
		k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
		if err != nil {
			return false
		}
		defer k.Close()
		names, _ := k.ReadValueNames(0)
		for _, n := range names {
			if strings.Trim(strings.ToLower(n), "{}") == cleanCLSID {
				return true
			}
		}
		return false
	}

	if checkKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Shell Extensions\Blocked`) {
		return true
	}
	if checkKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Shell Extensions\Blocked`) {
		return true
	}
	return false
}

// resolveCLSIDDll resolves a CLSID to its InprocServer32 DLL file path.
func resolveCLSIDDll(clsid string) (dllPath, friendlyName string) {
	if clsid == "" {
		return "", ""
	}
	clsidKeyPath := `CLSID\` + clsid
	k, err := registry.OpenKey(registry.CLASSES_ROOT, clsidKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return "", ""
	}
	defer k.Close()
	friendlyName, _, _ = k.GetStringValue("")

	sk, err := registry.OpenKey(k, "InprocServer32", registry.QUERY_VALUE)
	if err != nil {
		return "", friendlyName
	}
	defer sk.Close()

	dllPath, _, _ = sk.GetStringValue("")
	dllPath = os.ExpandEnv(strings.Trim(dllPath, `"`))
	return dllPath, friendlyName
}

func scanShellExtensions(pt procTable, _ bool) ([]Component, error) {
	var out []Component
	seen := map[string]bool{}

	for _, loc := range shellExLocations {
		k, err := registry.OpenKey(registry.CLASSES_ROOT, loc.path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		subkeys, _ := k.ReadSubKeyNames(0)
		for _, name := range subkeys {
			sk, err := registry.OpenKey(k, name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			val, _, _ := sk.GetStringValue("")
			sk.Close()

			clsid := val
			if clsid == "" && strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}") {
				clsid = name
			}
			if clsid == "" {
				continue
			}

			dedupKey := strings.ToLower(loc.path + "|" + name + "|" + clsid)
			if seen[dedupKey] {
				continue
			}
			seen[dedupKey] = true

			dllPath, friendlyName := resolveCLSIDDll(clsid)
			if dllPath == "" {
				continue
			}

			isBlocked := isCLSIDBlocked(clsid)
			enabled := !isBlocked

			displayName := name
			if friendlyName != "" {
				displayName = friendlyName
			}

			out = append(out, Component{
				ID:        "shellex:" + clsid,
				Kind:      "shellex",
				Name:      name,
				Display:   displayName,
				Command:   dllPath,
				ExePath:   dllPath,
				Source:    loc.label,
				Trigger:   "When right-clicking in File Explorer",
				StartType: "Dynamic DLL Load",
				Enabled:   enabled,
				AutoStart: false,
				Running:   false,
				tgt: Target{
					Kind: "shellex",
					Root: "HKCU",
					Path: `Software\Microsoft\Windows\CurrentVersion\Shell Extensions\Blocked`,
					Name: clsid,
				},
			})
		}
		k.Close()
	}
	return out, nil
}
