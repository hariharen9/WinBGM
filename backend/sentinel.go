package backend

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modAdvapi32Sentinel     = windows.NewLazySystemDLL("advapi32.dll")
	procRegNotifyChangeKey  = modAdvapi32Sentinel.NewProc("RegNotifyChangeKeyValue")
)

const (
	regNotifyChangeName    = 0x00000001
	regNotifyChangeLastSet = 0x00000004
	keyNotify              = 0x0010
)

type SentinelAlert struct {
	ID        string `json:"id"`
	RootKey   string `json:"rootKey"`
	SubKey    string `json:"subKey"`
	Name      string `json:"name"`
	Command   string `json:"command"`
	Timestamp string `json:"timestamp"`
	Action    string `json:"action"` // "re_enabled" or "added"
	Message   string `json:"message"`
}

type watchedLocation struct {
	root     registry.Key
	rootName string
	path     string
}

var defaultWatched = []watchedLocation{
	{root: registry.CURRENT_USER, rootName: "HKCU", path: `Software\Microsoft\Windows\CurrentVersion\Run`},
	{root: registry.LOCAL_MACHINE, rootName: "HKLM", path: `Software\Microsoft\Windows\CurrentVersion\Run`},
	{root: registry.CURRENT_USER, rootName: "HKCU", path: `Software\Microsoft\Windows\CurrentVersion\RunOnce`},
	{root: registry.LOCAL_MACHINE, rootName: "HKLM", path: `Software\Microsoft\Windows\CurrentVersion\RunOnce`},
}

var (
	sentinelMu     sync.Mutex
	sentinelAlerts []SentinelAlert
	knownValues    = map[string]string{}
	sentinelCancel context.CancelFunc
)

func snapshotKeyValues(root registry.Key, path string) map[string]string {
	res := map[string]string{}
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return res
	}
	defer k.Close()

	names, err := k.ReadValueNames(-1)
	if err != nil {
		return res
	}
	for _, n := range names {
		val, _, err := k.GetStringValue(n)
		if err == nil {
			res[n] = val
		}
	}
	return res
}

// StartSentinel begins background monitoring for persistence changes.
func StartSentinel(parentCtx context.Context, onAlert func(SentinelAlert)) {
	sentinelMu.Lock()
	if sentinelCancel != nil {
		sentinelMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parentCtx)
	sentinelCancel = cancel

	// Initialize baseline of known values
	for _, loc := range defaultWatched {
		vals := snapshotKeyValues(loc.root, loc.path)
		for k, v := range vals {
			key := fmt.Sprintf("%s\\%s:%s", loc.rootName, loc.path, k)
			knownValues[key] = v
		}
	}
	sentinelMu.Unlock()

	for _, loc := range defaultWatched {
		go watchRegistryKey(ctx, loc, onAlert)
	}
}

// StopSentinel halts the watcher goroutines.
func StopSentinel() {
	sentinelMu.Lock()
	defer sentinelMu.Unlock()
	if sentinelCancel != nil {
		sentinelCancel()
		sentinelCancel = nil
	}
}

func watchRegistryKey(ctx context.Context, loc watchedLocation, onAlert func(SentinelAlert)) {
	var hKey windows.Handle
	var rootHandle windows.Handle
	switch loc.root {
	case registry.CURRENT_USER:
		rootHandle = windows.HKEY_CURRENT_USER
	case registry.LOCAL_MACHINE:
		rootHandle = windows.HKEY_LOCAL_MACHINE
	default:
		return
	}

	pathUTF16, err := windows.UTF16PtrFromString(loc.path)
	if err != nil {
		return
	}

	access := uint32(windows.KEY_READ | keyNotify)
	err = windows.RegOpenKeyEx(rootHandle, pathUTF16, 0, access, &hKey)
	if err != nil {
		return
	}
	defer windows.RegCloseKey(hKey)

	hEvent, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return
	}
	defer windows.CloseHandle(hEvent)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Register for notifications
		r, _, _ := procRegNotifyChangeKey.Call(
			uintptr(hKey),
			0, // bWatchSubtree = FALSE
			uintptr(regNotifyChangeName|regNotifyChangeLastSet),
			uintptr(hEvent),
			1, // fAsynchronous = TRUE
		)
		if r != 0 {
			time.Sleep(5 * time.Second)
			continue
		}

		// Wait with 1-second timeout so context cancellation can be observed
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			waitRes, _ := windows.WaitForSingleObject(hEvent, 1000)
			if waitRes == windows.WAIT_OBJECT_0 {
				// Event signaled: key changed!
				checkForChanges(loc, onAlert)
				break
			}
		}
	}
}

func checkForChanges(loc watchedLocation, onAlert func(SentinelAlert)) {
	currentVals := snapshotKeyValues(loc.root, loc.path)

	sentinelMu.Lock()
	defer sentinelMu.Unlock()

	for name, val := range currentVals {
		key := fmt.Sprintf("%s\\%s:%s", loc.rootName, loc.path, name)
		prevVal, existed := knownValues[key]

		if !existed {
			knownValues[key] = val
			alert := SentinelAlert{
				ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
				RootKey:   loc.rootName,
				SubKey:    loc.path,
				Name:      name,
				Command:   val,
				Timestamp: time.Now().Format("15:04:05"),
				Action:    "added",
				Message:   fmt.Sprintf("Application entry %q was added to %s\\%s startup", name, loc.rootName, loc.path),
			}
			sentinelAlerts = append(sentinelAlerts, alert)
			if onAlert != nil {
				go onAlert(alert)
			}
		} else if prevVal != val {
			knownValues[key] = val
			alert := SentinelAlert{
				ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
				RootKey:   loc.rootName,
				SubKey:    loc.path,
				Name:      name,
				Command:   val,
				Timestamp: time.Now().Format("15:04:05"),
				Action:    "re_enabled",
				Message:   fmt.Sprintf("Application entry %q was modified or re-enabled in %s\\%s", name, loc.rootName, loc.path),
			}
			sentinelAlerts = append(sentinelAlerts, alert)
			if onAlert != nil {
				go onAlert(alert)
			}
		}
	}
}

// GetSentinelAlerts returns current alerts.
func GetSentinelAlerts() []SentinelAlert {
	sentinelMu.Lock()
	defer sentinelMu.Unlock()
	res := make([]SentinelAlert, len(sentinelAlerts))
	copy(res, sentinelAlerts)
	return res
}

// ClearSentinelAlerts clears all active alerts.
func ClearSentinelAlerts() {
	sentinelMu.Lock()
	defer sentinelMu.Unlock()
	sentinelAlerts = nil
}

// DismissSentinelAlert removes a single alert from the active list.
func DismissSentinelAlert(id string) {
	sentinelMu.Lock()
	defer sentinelMu.Unlock()
	filtered := sentinelAlerts[:0]
	for _, a := range sentinelAlerts {
		if a.ID != id {
			filtered = append(filtered, a)
		}
	}
	sentinelAlerts = filtered
}

// DisableSentinelAlert safely disables a newly detected Run-key startup entry via StartupApproved
// with full history backup, then dismisses the alert.
func DisableSentinelAlert(id string) (ChangeResult, error) {
	sentinelMu.Lock()
	var target *SentinelAlert
	for i := range sentinelAlerts {
		if sentinelAlerts[i].ID == id {
			cp := sentinelAlerts[i]
			target = &cp
			break
		}
	}
	sentinelMu.Unlock()

	if target == nil {
		return ChangeResult{}, fmt.Errorf("sentinel alert not found")
	}

	approvedPath := approvedBase + "Run"
	if strings.Contains(strings.ToUpper(target.SubKey), "RUNONCE") {
		approvedPath = approvedBase + "RunOnce"
	}

	tgt := Target{
		Kind: "run",
		Root: target.RootKey,
		Path: approvedPath,
		Name: target.Name,
	}

	// Build a temporary component to reuse Apply's full safety pipeline & history backup
	inv := BuildInventory()
	for _, app := range inv.Apps {
		for _, c := range app.Components {
			if c.Kind == "run" && strings.EqualFold(c.Name, target.Name) {
				res := Apply(&inv, ChangeRequest{
					ID:     c.ID,
					Action: "disable",
				})
				if res.OK {
					DismissSentinelAlert(id)
					return res, nil
				}
				return res, fmt.Errorf("%s", res.Message)
			}
		}
	}

	// Direct StartupApproved disable fallback if not yet in cached inventory
	_ = tgt
	return ChangeResult{}, fmt.Errorf("could not locate %q in live inventory; please run a scan and disable it from Applications", target.Name)
}
