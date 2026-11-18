package main

import "strings"

// ownerOf guesses which application owns an executable from where it is installed.
// Returns key, label and whether it is part of Windows itself.
func ownerOf(p string) (string, string, bool) {
	parts := strings.Split(strings.TrimRight(p, `\`), `\`)
	if len(parts) < 3 {
		return "", "", false
	}
	low := make([]string, len(parts))
	for i, s := range parts {
		low[i] = strings.ToLower(s)
	}
	if low[1] == "windows" {
		return "windows", "Windows", true
	}
	for i, s := range low {
		if s == "windowsapps" && i+1 < len(parts) {
			name := strings.SplitN(parts[i+1], "_", 2)[0]
			return "pkg:" + strings.ToLower(name), packageDisplay(name), isWindowsPackage(name)
		}
	}
	switch low[1] {
	case "program files", "program files (x86)", "programdata":
		if len(parts) > 3 {
			return "dir:" + low[2], parts[2], false
		}
	case "users":
		if len(parts) > 6 && low[3] == "appdata" {
			if low[4] == "local" && len(parts) > 7 && low[5] == "programs" {
				return "dir:" + low[6], parts[6], false
			}
			return "dir:" + low[5], parts[5], false
		}
	}
	d := parts[len(parts)-2]
	return "dir:" + strings.ToLower(d), d, false
}

// packageDisplay turns "5319275A.WhatsAppDesktop" into "WhatsAppDesktop" (version info usually gives a nicer name later).
func packageDisplay(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 && i+1 < len(name) {
		return name[i+1:]
	}
	return name
}

func isWindowsPackage(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "microsoftwindows.") || strings.HasPrefix(n, "windows.") ||
		strings.HasPrefix(n, "microsoft.windows.") || strings.HasPrefix(n, "microsoft.ui.") ||
		strings.HasPrefix(n, "microsoft.vclibs") || strings.HasPrefix(n, "microsoft.net.")
}
