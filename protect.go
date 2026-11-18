package main

import (
	"os"
	"regexp"
	"strings"
)

var criticalServices = map[string]bool{
	"windefend": true, "wscsvc": true, "securityhealthservice": true, "sense": true, "mpssvc": true,
	"bfe": true, "eventlog": true, "rpcss": true, "rpceptmapper": true, "dcomlaunch": true, "lsm": true,
	"samss": true, "winmgmt": true, "cryptsvc": true, "dhcp": true, "dnscache": true, "nsi": true,
	"plugplay": true, "power": true, "profsvc": true, "usermanager": true, "gpsvc": true, "schedule": true,
	"trustedinstaller": true, "wuauserv": true, "bits": true, "w32time": true, "audiosrv": true,
	"audioendpointbuilder": true, "themes": true, "lanmanserver": true, "lanmanworkstation": true,
	"netlogon": true, "keyiso": true, "vaultsvc": true, "sysmain": true, "appinfo": true,
}

var (
	securityRe  = regexp.MustCompile(`(?i)(antivirus|defender|kaspersky|mcafee|norton|avast|\bavg\b|bitdefender|malwarebytes|\beset\b|sophos|crowdstrike|sentinelone|windows security)`)
	antiCheatRe = regexp.MustCompile(`(?i)(\bvgc\b|\bvgk\b|vanguard|easyanticheat|\bbeservice\b|battleye|faceit|\besea\b)`)
	driverRe    = regexp.MustCompile(`(?i)(nvidia|advanced micro devices|\bamd\b|intel|realtek|logitech|razer)`)
)

func selfExe() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return strings.ToLower(p)
}

// protectedReason returns why a component must never be changed from this app ("" = allowed).
func protectedReason(c *Component) string {
	low := strings.ToLower(c.ExePath)
	switch {
	case c.System || strings.Contains(low, `\windows\`):
		return "Part of Windows. Background Manager never changes Windows' own components."
	case c.Kind == "service" && criticalServices[strings.ToLower(c.Name)]:
		return "Critical Windows service. Changing it can break sign-in, networking, updates or security."
	case securityRe.MatchString(c.Name + " " + c.Display + " " + c.Product):
		return "Security software. Changing it could leave this PC unprotected."
	case low != "" && low == selfExe():
		return "This is Background Manager itself."
	}
	return ""
}

func cautionReason(c *Component) string {
	all := c.Name + " " + c.Display + " " + c.Product + " " + c.ExePath
	switch {
	case antiCheatRe.MatchString(all):
		return "This looks like game anti-cheat software. Disabling it can stop protected games from launching."
	case c.Kind == "service" && driverRe.MatchString(c.Publisher+" "+c.Product):
		return "Driver-related service. Changing it can affect graphics, audio or device features."
	case c.Kind == "service":
		return "Other software may depend on this service."
	}
	return ""
}

func needsAdmin(c *Component) bool {
	return c.Kind == "service" || c.Kind == "task" || c.tgt.Root == "HKLM"
}

func computeControl(c *Component, elevated bool) {
	switch c.Kind {
	case "process", "storeapp":
		c.Control, c.ControlNote = "no", "This can't be changed safely from here. See \u201CHow to change\u201D."
		return
	}
	if r := protectedReason(c); r != "" {
		c.Control, c.ControlNote = "no", r
		return
	}
	c.Caution = cautionReason(c)
	c.CanStop = c.Kind == "service"
	switch {
	case c.Locked == "yes":
		c.Control, c.ControlNote = "no", "Locked by its publisher: Windows doesn't let this account change its startup type. You can still stop or start it."
	case needsAdmin(c) && !elevated:
		c.Control, c.ControlNote = "admin", "Needs administrator rights. Use \u201CRestart as administrator\u201D."
	default:
		c.Control = "yes"
	}
}
