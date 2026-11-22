package backend

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Target identifies exactly what is changed. It is stored in the history so a restore never depends on a fresh scan.
type Target struct {
	Kind string `json:"kind"` // run | folder | task | service | servicerun
	Root string `json:"root"` // HKCU | HKLM (run, folder)
	Path string `json:"path"` // StartupApproved subkey, or task folder
	Name string `json:"name"`
}

type snapshot struct {
	State   string
	Raw     []byte
	Existed bool
}

type ChangeRequest struct {
	ID     string `json:"id"`
	Action string `json:"action"` // disable | enable | stop | start
}

type ChangeResult struct {
	OK      bool          `json:"ok"`
	Message string        `json:"message"`
	Entry   *HistoryEntry `json:"entry,omitempty"`
}

var opMu sync.Mutex

func fail(m string) ChangeResult { return ChangeResult{Message: m} }

func psq(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func rootKey(r string) registry.Key {
	if r == "HKLM" {
		return registry.LOCAL_MACHINE
	}
	return registry.CURRENT_USER
}

func rootName(k registry.Key) string {
	if k == registry.LOCAL_MACHINE {
		return "HKLM"
	}
	return "HKCU"
}

// ---------- reading state ----------

func readTarget(t Target) (snapshot, error) {
	switch t.Kind {
	case "run", "folder":
		k, err := registry.OpenKey(rootKey(t.Root), t.Path, registry.QUERY_VALUE)
		if err != nil {
			return snapshot{State: "on"}, nil
		}
		defer k.Close()
		v, _, err := k.GetBinaryValue(t.Name)
		if err != nil || len(v) == 0 {
			return snapshot{State: "on"}, nil
		}
		s := snapshot{State: "on", Raw: v, Existed: true}
		if v[0]&1 == 1 {
			s.State = "off"
		}
		return s, nil
	case "task":
		out, err := ps(`(Get-ScheduledTask -TaskPath ` + psq(t.Path) + ` -TaskName ` + psq(t.Name) + ` -ErrorAction Stop).State`)
		if err != nil {
			return snapshot{}, psErr(err)
		}
		if strings.TrimSpace(string(out)) == "Disabled" {
			return snapshot{State: "disabled"}, nil
		}
		return snapshot{State: "enabled"}, nil
	case "service", "servicerun":
		start, running, err := readService(t.Name)
		if err != nil {
			return snapshot{}, err
		}
		if t.Kind == "servicerun" {
			if running {
				return snapshot{State: "running"}, nil
			}
			return snapshot{State: "stopped"}, nil
		}
		return snapshot{State: start}, nil
	}
	return snapshot{}, errors.New("unsupported item")
}

func readService(name string) (start string, running bool, err error) {
	m, err := openSCM(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return "", false, err
	}
	defer m.Disconnect()
	s, err := openSvc(m, name, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return "", false, err
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return "", false, err
	}
	st, err := s.Query()
	if err != nil {
		return "", false, err
	}
	switch cfg.StartType {
	case mgr.StartAutomatic:
		start = "auto"
		if cfg.DelayedAutoStart {
			start = "delayed"
		}
	case mgr.StartDisabled:
		start = "disabled"
	default:
		start = "manual"
	}
	return start, st.State == svc.Running, nil
}

func runningDependents(name string) []string {
	m, err := openSCM(windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil
	}
	defer m.Disconnect()
	s, err := openSvc(m, name, windows.SERVICE_ENUMERATE_DEPENDENTS)
	if err != nil {
		return nil
	}
	defer s.Close()
	l, _ := s.ListDependentServices(svc.Active)
	return l
}

// ---------- writing state ----------

func scRun(args ...string) (int, string) {
	c := exec.Command("sc.exe", args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := c.CombinedOutput()
	if err == nil {
		return 0, ""
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), strings.TrimSpace(string(out))
	}
	return 1, err.Error()
}

func scErr(code int, out string) error {
	if code == 5 {
		return syscall.ERROR_ACCESS_DENIED
	}
	return fmt.Errorf("%s", out)
}

func waitService(name string, wantRunning bool) error {
	for i := 0; i < 16; i++ {
		_, running, err := readService(name)
		if err == nil && running == wantRunning {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("the service did not reach the expected state in time")
}

func writeApproved(t Target, on bool) error {
	k, _, err := registry.CreateKey(rootKey(t.Root), t.Path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	buf := make([]byte, 12)
	if on {
		buf[0] = 2
	} else {
		buf[0] = 3
		ft := windows.NsecToFiletime(time.Now().UnixNano())
		binary.LittleEndian.PutUint32(buf[4:], ft.LowDateTime)
		binary.LittleEndian.PutUint32(buf[8:], ft.HighDateTime)
	}
	return k.SetBinaryValue(t.Name, buf)
}

func writeTarget(t Target, desired string) error {
	switch t.Kind {
	case "run", "folder":
		return writeApproved(t, desired == "on")
	case "task":
		verb := "Disable-ScheduledTask"
		if desired == "enabled" {
			verb = "Enable-ScheduledTask"
		}
		_, err := ps(verb + ` -TaskPath ` + psq(t.Path) + ` -TaskName ` + psq(t.Name) + ` -ErrorAction Stop | Out-Null`)
		return psErr(err)
	case "service":
		arg := map[string]string{"auto": "auto", "delayed": "delayed-auto", "manual": "demand", "disabled": "disabled"}[desired]
		if arg == "" {
			return errors.New("unsupported startup type")
		}
		if code, out := scRun("config", t.Name, "start=", arg); code != 0 {
			return scErr(code, out)
		}
		return nil
	case "servicerun":
		if desired == "stopped" {
			if code, out := scRun("stop", t.Name); code != 0 && code != 1062 { // 1062: not started
				return scErr(code, out)
			}
			return waitService(t.Name, false)
		}
		if code, out := scRun("start", t.Name); code != 0 && code != 1056 { // 1056: already running
			return scErr(code, out)
		}
		return waitService(t.Name, true)
	}
	return errors.New("unsupported item")
}

// restoreTarget puts the exact previous value back (registry items restore the original bytes, or remove the value).
func restoreTarget(t Target, before snapshot) error {
	if t.Kind != "run" && t.Kind != "folder" {
		return writeTarget(t, before.State)
	}
	k, err := registry.OpenKey(rootKey(t.Root), t.Path, registry.SET_VALUE)
	if err != nil {
		if !before.Existed {
			return nil
		}
		return err
	}
	defer k.Close()
	if before.Existed {
		return k.SetBinaryValue(t.Name, before.Raw)
	}
	if err := k.DeleteValue(t.Name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func psErr(err error) error {
	if err == nil {
		return nil
	}
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return errors.New(strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}

func friendly(err error) string {
	s := strings.ToLower(err.Error())
	if errors.Is(err, syscall.ERROR_ACCESS_DENIED) || strings.Contains(s, "denied") {
		return "Windows denied the change. It needs administrator rights, or the item is locked by its publisher."
	}
	return "Windows reported: " + strings.TrimSpace(err.Error())
}

// ---------- labels ----------

func humanState(kind, s string) string {
	switch kind {
	case "run", "folder":
		if s == "on" {
			return "Starts at sign-in"
		}
		return "Doesn't start at sign-in"
	case "task":
		if s == "enabled" {
			return "Enabled"
		}
		return "Disabled"
	case "service":
		return map[string]string{"auto": "Automatic", "delayed": "Automatic (Delayed)", "manual": "Manual", "disabled": "Disabled"}[s]
	}
	if s == "running" {
		return "Running"
	}
	return "Stopped"
}

func summaryOf(kind, before, after string) string {
	label := map[string]string{"run": "Launch", "folder": "Launch", "task": "Scheduled task", "service": "Startup type", "servicerun": "Service"}[kind]
	return fmt.Sprintf("%s: %s \u2192 %s", label, humanState(kind, before), humanState(kind, after))
}

func scanStateOf(c *Component, kind string) string {
	switch kind {
	case "run", "folder":
		if c.Enabled {
			return "on"
		}
		return "off"
	case "task":
		if c.Enabled {
			return "enabled"
		}
		return "disabled"
	case "servicerun":
		if c.Running {
			return "running"
		}
		return "stopped"
	}
	return map[string]string{"Automatic": "auto", "Automatic (Delayed)": "delayed", "Manual": "manual", "Disabled": "disabled"}[c.StartType]
}

func findComponent(inv *Inventory, id string) (*Component, string) {
	for i := range inv.Apps {
		for j := range inv.Apps[i].Components {
			if inv.Apps[i].Components[j].ID == id {
				return &inv.Apps[i].Components[j], inv.Apps[i].Name
			}
		}
	}
	return nil, ""
}

func targetNeedsAdmin(t Target) bool {
	return t.Kind == "service" || t.Kind == "task" || t.Root == "HKLM"
}

// ---------- the operations the UI can call ----------

// Apply changes one item after re-checking every safety rule on the Go side (the UI is never trusted).
func Apply(last *Inventory, req ChangeRequest) ChangeResult {
	opMu.Lock()
	defer opMu.Unlock()
	if last == nil {
		return fail("Scan first.")
	}
	c, appName := findComponent(last, req.ID)
	if c == nil {
		return fail("That item is no longer in the scan. Scan again.")
	}
	isStop := req.Action == "stop" || req.Action == "start"
	if isStop {
		if c.Kind != "service" || !c.CanStop {
			return fail(orElse(c.ControlNote, "This service can't be stopped from here."))
		}
	} else if c.Control != "yes" {
		if c.Control == "admin" && isElevated() {
			// elevated since the scan: fall through to the live checks below
		} else {
			return fail(orElse(c.ControlNote, "This item can't be changed from here."))
		}
	}
	if r := protectedReason(c); r != "" {
		return fail(r)
	}
	if !isStop && needsAdmin(c) && !isElevated() {
		return fail("This change needs administrator rights. Use \u201CRestart as administrator\u201D.")
	}

	tgt := c.tgt
	var desired string
	switch c.Kind {
	case "run", "folder":
		desired = map[string]string{"disable": "off", "enable": "on"}[req.Action]
	case "task":
		desired = map[string]string{"disable": "disabled", "enable": "enabled"}[req.Action]
	case "service":
		switch req.Action {
		case "disable":
			desired = "manual"
		case "enable":
			desired = "auto"
		case "stop":
			tgt.Kind, desired = "servicerun", "stopped"
		case "start":
			tgt.Kind, desired = "servicerun", "running"
		}
	}
	if desired == "" {
		return fail("Unsupported action.")
	}

	cur, err := readTarget(tgt)
	if err != nil {
		return fail("Couldn't read the current setting: " + friendly(err))
	}
	if cur.State != scanStateOf(c, tgt.Kind) {
		return fail("This item changed since the last scan. Scan again before changing it.")
	}
	if cur.State == desired {
		return fail("It is already set that way.")
	}
	if tgt.Kind == "servicerun" && desired == "stopped" {
		if dep := runningDependents(tgt.Name); len(dep) > 0 {
			return fail("Not stopped: these running services depend on it: " + strings.Join(dep, ", ") + ".")
		}
	}

	e := HistoryEntry{
		ID: fmt.Sprint(time.Now().UnixNano()), Time: time.Now().Format(time.RFC3339),
		Title: orElse(c.Display, c.Name), AppName: appName, Kind: tgt.Kind, Action: req.Action,
		Summary: summaryOf(tgt.Kind, cur.State, desired), Target: tgt, Before: cur.State, After: desired,
		BeforeRaw: hex.EncodeToString(cur.Raw), BeforeExisted: cur.Existed, Status: "pending",
	}
	// The backup is written BEFORE touching anything; if it can't be saved, nothing changes.
	if err := appendHistory(e); err != nil {
		return fail("Couldn't save a backup, so nothing was changed: " + err.Error())
	}
	if err := writeTarget(tgt, desired); err != nil {
		e.Status, e.Error = "failed", friendly(err)
		updateHistory(e)
		return ChangeResult{Message: e.Error, Entry: &e}
	}
	if after, err := readTarget(tgt); err != nil || after.State != desired {
		_ = restoreTarget(tgt, cur)
		e.Status, e.Error = "rolledback", "The change did not take effect, so the original setting was put back."
		updateHistory(e)
		return ChangeResult{Message: e.Error, Entry: &e}
	}
	e.Status, e.Restorable = "applied", true
	updateHistory(e)
	return ChangeResult{OK: true, Message: "Done: " + e.Summary + ". You can undo it in Change history.", Entry: &e}
}

// Restore undoes one history entry, but only if the item is still exactly as WinBGM left it.
func Restore(id string) ChangeResult {
	opMu.Lock()
	defer opMu.Unlock()
	var e *HistoryEntry
	for _, h := range loadHistory() {
		if h.ID == id {
			h := h
			e = &h
		}
	}
	if e == nil || e.Status != "applied" || !e.Restorable {
		return fail("This change can't be restored.")
	}
	if targetNeedsAdmin(e.Target) && !isElevated() && e.Target.Kind != "servicerun" {
		return fail("Restoring this needs administrator rights. Use \u201CRestart as administrator\u201D.")
	}
	cur, err := readTarget(e.Target)
	if err != nil {
		return fail("Couldn't read the current setting: " + friendly(err))
	}
	if cur.State != e.After {
		return fail("It was changed again after WinBGM's change (now: " + humanState(e.Target.Kind, cur.State) + "). Restore was skipped so nothing is overwritten.")
	}
	raw, _ := hex.DecodeString(e.BeforeRaw)
	before := snapshot{State: e.Before, Raw: raw, Existed: e.BeforeExisted}
	if err := restoreTarget(e.Target, before); err != nil {
		return fail(friendly(err))
	}
	if after, err := readTarget(e.Target); err != nil || after.State != e.Before {
		return fail("The restore did not take effect. Check the item in Windows' own tools.")
	}
	e.Status, e.Restorable, e.RestoredAt = "restored", false, time.Now().Format(time.RFC3339)
	updateHistory(*e)
	return ChangeResult{OK: true, Message: "Restored: " + humanState(e.Target.Kind, e.Before) + ".", Entry: e}
}

// History returns past changes, newest first.
func History() []HistoryEntry {
	h := loadHistory()
	for i, j := 0, len(h)-1; i < j; i, j = i+1, j-1 {
		h[i], h[j] = h[j], h[i]
	}
	return h
}

// RelaunchAsAdmin relaunches this exe with a UAC prompt.
func RelaunchAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	return windows.ShellExecute(0, verb, file, nil, nil, 1)
}

func orElse(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

type BatchResult struct {
	Total    int      `json:"total"`
	Modified int      `json:"modified"`
	Failed   int      `json:"failed"`
	Message  string   `json:"message"`
	Entries  []string `json:"entries"`
}

// ApplyFocusMode disables third-party automatic startup items safely.
// Each item is backed up individually to history first and can be undone at any time.
func ApplyFocusMode(last *Inventory) BatchResult {
	if last == nil {
		return BatchResult{Message: "Run a scan first."}
	}
	var toDisable []ChangeRequest
	for _, app := range last.Apps {
		if app.System {
			continue
		}
		for _, c := range app.Components {
			if c.AutoStart && c.Control == "yes" && !c.System && c.Kind != "process" && c.Kind != "storeapp" {
				toDisable = append(toDisable, ChangeRequest{ID: c.ID, Action: "disable"})
			}
		}
	}
	if len(toDisable) == 0 {
		return BatchResult{Message: "No third-party automatic startup items to disable."}
	}

	res := BatchResult{Total: len(toDisable)}
	for _, req := range toDisable {
		r := Apply(last, req)
		if r.OK {
			res.Modified++
			if r.Entry != nil {
				res.Entries = append(res.Entries, r.Entry.ID)
			}
		} else {
			res.Failed++
		}
	}
	res.Message = fmt.Sprintf("Focus Mode: %d item(s) safely disabled and backed up. You can restore them anytime from Change history.", res.Modified)
	return res
}

