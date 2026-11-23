package backend

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type appxManifest struct {
	XMLName    xml.Name `xml:"Package"`
	Identity   struct {
		Name      string `xml:"Name,attr"`
		Publisher string `xml:"Publisher,attr"`
		Version   string `xml:"Version,attr"`
	} `xml:"Identity"`
	Properties struct {
		DisplayName          string `xml:"DisplayName"`
		PublisherDisplayName string `xml:"PublisherDisplayName"`
		Logo                 string `xml:"Logo"`
	} `xml:"Properties"`
	Applications struct {
		Application []struct {
			ID             string `xml:"Id,attr"`
			Executable     string `xml:"Executable,attr"`
			VisualElements struct {
				DisplayName string `xml:"DisplayName,attr"`
			} `xml:"VisualElements"`
		} `xml:"Application"`
	} `xml:"Applications"`
}

type AppxInfo struct {
	PackageName string `json:"packageName"`
	DisplayName string `json:"displayName"`
	Publisher   string `json:"publisher"`
	Version     string `json:"version"`
	IsMicrosoft bool   `json:"isMicrosoft"`
}

var (
	appxCacheMu sync.RWMutex
	appxCache   = map[string]AppxInfo{}
)

// ResolveAppxFromPath checks if an exe path belongs to a Windows Store / MSIX package under WindowsApps,
// and parses its AppxManifest.xml to extract the official human-readable display name and publisher.
func ResolveAppxFromPath(exePath string) (AppxInfo, bool) {
	norm := filepath.Clean(exePath)
	parts := strings.Split(norm, string(filepath.Separator))
	pkgDir := ""
	for i, part := range parts {
		if strings.EqualFold(part, "WindowsApps") && i+1 < len(parts) {
			pkgDir = strings.Join(parts[:i+2], string(filepath.Separator))
			break
		}
	}
	if pkgDir == "" {
		return AppxInfo{}, false
	}

	appxCacheMu.RLock()
	info, found := appxCache[pkgDir]
	appxCacheMu.RUnlock()
	if found {
		return info, true
	}

	manifestPath := filepath.Join(pkgDir, "AppxManifest.xml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		base := filepath.Base(pkgDir)
		name := strings.SplitN(base, "_", 2)[0]
		info = AppxInfo{
			PackageName: name,
			DisplayName: packageDisplay(name),
			IsMicrosoft: isWindowsPackage(name),
		}
		appxCacheMu.Lock()
		appxCache[pkgDir] = info
		appxCacheMu.Unlock()
		return info, true
	}

	var m appxManifest
	if err := xml.Unmarshal(data, &m); err != nil {
		base := filepath.Base(pkgDir)
		name := strings.SplitN(base, "_", 2)[0]
		info = AppxInfo{
			PackageName: name,
			DisplayName: packageDisplay(name),
			IsMicrosoft: isWindowsPackage(name),
		}
		return info, true
	}

	displayName := m.Properties.DisplayName
	if displayName == "" || strings.HasPrefix(displayName, "ms-resource:") {
		for _, app := range m.Applications.Application {
			if app.VisualElements.DisplayName != "" && !strings.HasPrefix(app.VisualElements.DisplayName, "ms-resource:") {
				displayName = app.VisualElements.DisplayName
				break
			}
		}
	}
	if displayName == "" || strings.HasPrefix(displayName, "ms-resource:") {
		displayName = packageDisplay(m.Identity.Name)
	}

	publisher := m.Properties.PublisherDisplayName
	if publisher == "" {
		publisher = m.Identity.Publisher
	}

	isMs := isWindowsPackage(m.Identity.Name) || strings.Contains(strings.ToLower(publisher), "microsoft")

	info = AppxInfo{
		PackageName: m.Identity.Name,
		DisplayName: displayName,
		Publisher:   publisher,
		Version:     m.Identity.Version,
		IsMicrosoft: isMs,
	}

	appxCacheMu.Lock()
	appxCache[pkgDir] = info
	appxCacheMu.Unlock()

	return info, true
}
