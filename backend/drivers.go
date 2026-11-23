package backend

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func scanDrivers(_ procTable, _ bool) ([]Component, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services`, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()

	names, err := k.ReadSubKeyNames(0)
	if err != nil {
		return nil, err
	}

	var out []Component
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}

	for _, name := range names {
		sk, err := registry.OpenKey(k, name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		typ, _, err := sk.GetIntegerValue("Type")
		if err != nil || (typ != 1 && typ != 2) { // 1 = Kernel Driver, 2 = File System Driver
			sk.Close()
			continue
		}

		startMode, _, _ := sk.GetIntegerValue("Start")
		imagePath, _, _ := sk.GetStringValue("ImagePath")
		dispName, _, _ := sk.GetStringValue("DisplayName")
		sk.Close()

		if imagePath == "" {
			continue
		}

		// Normalize image path
		cleanPath := strings.TrimSpace(imagePath)
		cleanPath = strings.TrimPrefix(cleanPath, `\??\`)
		cleanPath = strings.Trim(cleanPath, `"`)
		if strings.HasPrefix(strings.ToLower(cleanPath), `system32\`) {
			cleanPath = filepath.Join(sysRoot, cleanPath)
		} else if !filepath.IsAbs(cleanPath) && !strings.Contains(cleanPath, `\`) {
			cleanPath = filepath.Join(sysRoot, "System32", "drivers", cleanPath)
		}
		cleanPath = os.ExpandEnv(cleanPath)

		// Filter: focus on non-standard/third-party drivers or active hardware drivers
		lowPath := strings.ToLower(cleanPath)
		lowName := strings.ToLower(name)

		isThirdParty := !strings.HasPrefix(lowPath, strings.ToLower(filepath.Join(sysRoot, "system32", "drivers"))) ||
			strings.Contains(lowPath, "program files") ||
			strings.Contains(lowPath, "appdata") ||
			strings.Contains(lowPath, "corsair") ||
			strings.Contains(lowPath, "razer") ||
			strings.Contains(lowPath, "logitech") ||
			strings.Contains(lowPath, "steelseries") ||
			strings.Contains(lowPath, "riot") ||
			strings.Contains(lowPath, "vgc") ||
			strings.Contains(lowPath, "vgk") ||
			strings.Contains(lowPath, "easyanticheat") ||
			strings.Contains(lowPath, "battleye") ||
			strings.Contains(lowName, "vgc") ||
			strings.Contains(lowName, "vgk") ||
			strings.Contains(lowName, "easyanticheat") ||
			strings.Contains(lowName, "bedaisy")

		if !isThirdParty {
			// Skip generic in-box Windows kernel drivers to keep view clean
			continue
		}

		startText := "Demand / Manual"
		enabled := startMode != 4
		autoStart := false
		switch startMode {
		case 0:
			startText = "Boot start (Kernel core)"
			autoStart = enabled
		case 1:
			startText = "System start (OS init)"
			autoStart = enabled
		case 2:
			startText = "Automatic"
			autoStart = enabled
		case 3:
			startText = "Manual / On-demand"
		case 4:
			startText = "Disabled"
		}

		title := dispName
		if title == "" {
			title = name
		}

		out = append(out, Component{
			ID:        "driver:" + name,
			Kind:      "driver",
			Name:      name,
			Display:   title,
			Command:   cleanPath,
			ExePath:   cleanPath,
			Source:    "Kernel Drivers",
			Trigger:   startText,
			StartType: startText,
			Enabled:   enabled,
			AutoStart: autoStart,
			Running:   enabled && autoStart, // Kernel drivers don't have user PIDs
			tgt: Target{
				Kind: "driver",
				Root: "HKLM",
				Path: `SYSTEM\CurrentControlSet\Services\` + name,
				Name: "Start",
			},
		})
	}

	return out, nil
}
