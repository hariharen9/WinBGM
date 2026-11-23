package backend

import (
	"encoding/xml"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

type BootDegradationItem struct {
	EventID        int    `json:"eventId"`
	Type           string `json:"type"` // "app" | "service" | "driver" | "task"
	Name           string `json:"name"`
	Path           string `json:"path"`
	TotalTimeMs    int64  `json:"totalTimeMs"`
	DelayMs        int64  `json:"delayMs"`
	FormattedDelay string `json:"formattedDelay"` // e.g. "+3.2s"
}

type BootProfile struct {
	Available      bool                  `json:"available"`
	RequiresAdmin  bool                  `json:"requiresAdmin"`
	BootDurationMs int64                 `json:"bootDurationMs"`
	MainPathMs     int64                 `json:"mainPathMs"`
	BootFormatted  string                `json:"bootFormatted"`
	BootTime       string                `json:"bootTime"`
	Degradations   []BootDegradationItem `json:"degradations"`
	Summary        string                `json:"summary"`
	Error          string                `json:"error,omitempty"`
}

type xmlEvent struct {
	System struct {
		EventID     int `xml:"EventID"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
	} `xml:"System"`
	EventData struct {
		Data []struct {
			Name  string `xml:"Name,attr"`
			Value string `xml:",chardata"`
		} `xml:"Data"`
	} `xml:"EventData"`
}

type xmlEvents struct {
	Events []xmlEvent `xml:"Event"`
}

var (
	modKernel32     = windows.NewLazySystemDLL("kernel32.dll")
	pGetTickCount64 = modKernel32.NewProc("GetTickCount64")
)

func getSystemUptime() time.Duration {
	r, _, _ := pGetTickCount64.Call()
	return time.Duration(r) * time.Millisecond
}

// GetBootProfile queries the Windows Event Log channel Microsoft-Windows-Diagnostics-Performance/Operational.
// Requires administrator elevation to access diagnostic channels.
func GetBootProfile() BootProfile {
	elevated := isElevated()
	if !elevated {
		return BootProfile{
			Available:     false,
			RequiresAdmin: true,
			Summary:       "Administrator rights are required to read Windows boot diagnostic logs.",
		}
	}

	// Query latest 50 events from Diagnostics-Performance directly without complex XPath to avoid syntax errors
	cmd := exec.Command("wevtutil", "qe", "Microsoft-Windows-Diagnostics-Performance/Operational",
		"/c:50", "/rd:true", "/f:xml")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	out, err := cmd.Output()
	if err != nil {
		uptime := getSystemUptime()
		bootTime := time.Now().Add(-uptime)
		hours := int(uptime.Hours())
		mins := int(uptime.Minutes()) % 60
		return BootProfile{
			Available: true,
			Summary: fmt.Sprintf("PC running for %dh %dm (booted %s). Diagnostics log could not be queried.",
				hours, mins, bootTime.Format("Jan 02, 3:04 PM")),
			Error: "Query returned: " + err.Error(),
		}
	}

	// Wrap in a root element so encoding/xml can parse multiple <Event> nodes
	wrapped := "<Events>" + string(out) + "</Events>"
	var parsed xmlEvents
	_ = xml.Unmarshal([]byte(wrapped), &parsed)

	profile := BootProfile{Available: true}

	// Find the most recent Event 100
	var lastBootTime time.Time
	for _, ev := range parsed.Events {
		if ev.System.EventID == 100 {
			dataMap := make(map[string]string)
			for _, d := range ev.EventData.Data {
				dataMap[d.Name] = d.Value
			}

			if durStr, ok := dataMap["BootDuration"]; ok {
				if d, err := strconv.ParseInt(durStr, 10, 64); err == nil {
					profile.BootDurationMs = d
					profile.BootFormatted = fmt.Sprintf("%.1fs", float64(d)/1000.0)
				}
			}
			if mpStr, ok := dataMap["MainPathBootTime"]; ok {
				if mp, err := strconv.ParseInt(mpStr, 10, 64); err == nil {
					profile.MainPathMs = mp
				}
			}
			profile.BootTime = ev.System.TimeCreated.SystemTime
			if t, err := time.Parse(time.RFC3339Nano, profile.BootTime); err == nil {
				lastBootTime = t
			}
			break
		}
	}

	// Correlate degradations (Event IDs 101-109)
	for _, ev := range parsed.Events {
		id := ev.System.EventID
		if id < 101 || id > 109 {
			continue
		}

		if !lastBootTime.IsZero() {
			if evTime, err := time.Parse(time.RFC3339Nano, ev.System.TimeCreated.SystemTime); err == nil {
				diff := evTime.Sub(lastBootTime)
				if diff < -10*time.Minute || diff > 15*time.Minute {
					continue // Event from a previous boot session
				}
			}
		}

		dataMap := make(map[string]string)
		for _, d := range ev.EventData.Data {
			dataMap[d.Name] = d.Value
		}

		var item BootDegradationItem
		item.EventID = id
		switch id {
		case 101:
			item.Type = "app"
			item.Name = dataMap["Name"]
			item.Path = dataMap["Path"]
		case 102:
			item.Type = "driver"
			item.Name = dataMap["FriendlyName"]
			if item.Name == "" {
				item.Name = dataMap["ImageName"]
			}
		case 103:
			item.Type = "service"
			item.Name = dataMap["Name"]
			item.Path = dataMap["Path"]
		case 106:
			item.Type = "task"
			item.Name = dataMap["TaskName"]
		default:
			item.Type = "system"
			item.Name = dataMap["Name"]
		}

		if totStr, ok := dataMap["TotalTime"]; ok {
			item.TotalTimeMs, _ = strconv.ParseInt(totStr, 10, 64)
		}
		if degStr, ok := dataMap["DegradationTime"]; ok {
			item.DelayMs, _ = strconv.ParseInt(degStr, 10, 64)
		} else {
			item.DelayMs = item.TotalTimeMs
		}

		if item.DelayMs > 0 {
			if item.DelayMs >= 1000 {
				item.FormattedDelay = fmt.Sprintf("+%.1fs", float64(item.DelayMs)/1000.0)
			} else {
				item.FormattedDelay = fmt.Sprintf("+%dms", item.DelayMs)
			}
		}

		if item.Name != "" {
			profile.Degradations = append(profile.Degradations, item)
		}
	}

	uptime := getSystemUptime()
	bootTime := time.Now().Add(-uptime)
	hours := int(uptime.Hours())
	mins := int(uptime.Minutes()) % 60
	uptimeStr := fmt.Sprintf("%dh %dm", hours, mins)
	if hours == 0 {
		uptimeStr = fmt.Sprintf("%dm", mins)
	}

	if profile.BootDurationMs > 0 {
		var parts []string
		for _, d := range profile.Degradations {
			if d.DelayMs >= 500 {
				parts = append(parts, fmt.Sprintf("%s (%s)", d.Name, d.FormattedDelay))
			}
		}
		if len(parts) > 0 {
			profile.Summary = fmt.Sprintf("Last boot: %s. Notable delays: %s.", profile.BootFormatted, strings.Join(parts, ", "))
		} else {
			profile.Summary = fmt.Sprintf("Last boot: %s (no major application delays detected).", profile.BootFormatted)
		}
	} else {
		profile.Summary = fmt.Sprintf("PC running for %s (booted %s). No boot degradation bottlenecks recorded by Windows.",
			uptimeStr, bootTime.Format("Jan 02, 3:04 PM"))
	}

	return profile
}
