package backend

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modPsapi       = windows.NewLazySystemDLL("psapi.dll")
	pGetProcessMem = modPsapi.NewProc("GetProcessMemoryInfo")
)

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func getProcessMemory(pid uint32) uint64 {
	if pid == 0 {
		return 0
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)

	var counters processMemoryCounters
	counters.CB = uint32(unsafe.Sizeof(counters))
	r, _, _ := pGetProcessMem.Call(uintptr(h), uintptr(unsafe.Pointer(&counters)), uintptr(counters.CB))
	if r == 0 {
		return 0
	}
	return uint64(counters.WorkingSetSize)
}

type procInfo struct {
	PID, PPID  uint32
	Name, Path string
	Window     bool // owns at least one visible top-level window
	RAM        uint64
}

type procTable struct {
	list   []procInfo
	byPath map[string][]uint32
	byPID  map[uint32]procInfo
}

type ProcessPollResult struct {
	TotalRAMBytes     uint64                         `json:"totalRAMBytes"`
	TotalRAMFormatted string                         `json:"totalRAMFormatted"`
	RunningPIDs       []uint32                       `json:"runningPIDs"`
	ActiveExePaths    map[string][]uint32            `json:"activeExePaths"`
	RAMByExePath      map[string]uint64              `json:"ramByExePath"`
	RAMFormattedByExe map[string]string              `json:"ramFormattedByExe"`
	RAMByPID          map[uint32]uint64              `json:"ramByPID"`
	ConnsByPID        map[uint32][]NetworkConnection `json:"connsByPID"`
}

func PollProcesses() ProcessPollResult {
	pt := snapshotProcs()
	var totalRAM uint64
	activePaths := make(map[string][]uint32)
	ramByPath := make(map[string]uint64)
	ramFmtByPath := make(map[string]string)
	ramByPID := make(map[uint32]uint64, len(pt.list))
	pids := make([]uint32, 0, len(pt.list))

	for _, p := range pt.list {
		pids = append(pids, p.PID)
		totalRAM += p.RAM
		ramByPID[p.PID] = p.RAM
		if p.Path != "" {
			low := strings.ToLower(p.Path)
			activePaths[low] = append(activePaths[low], p.PID)
			ramByPath[low] += p.RAM
		}
	}
	for low, bytes := range ramByPath {
		ramFmtByPath[low] = formatRAM(bytes)
	}

	connsByPID := GetConnectionsByPID()

	return ProcessPollResult{
		TotalRAMBytes:     totalRAM,
		TotalRAMFormatted: formatRAM(totalRAM),
		RunningPIDs:       pids,
		ActiveExePaths:    activePaths,
		RAMByExePath:      ramByPath,
		RAMFormattedByExe: ramFmtByPath,
		RAMByPID:          ramByPID,
		ConnsByPID:        connsByPID,
	}
}

func snapshotProcs() procTable {
	pt := procTable{byPath: map[string][]uint32{}, byPID: map[uint32]procInfo{}}
	wins := visibleWindowPIDs()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return pt
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if windows.Process32First(snap, &pe) != nil {
		return pt
	}
	for {
		p := procInfo{
			PID:    pe.ProcessID,
			PPID:   pe.ParentProcessID,
			Name:   windows.UTF16ToString(pe.ExeFile[:]),
			Window: wins[pe.ProcessID],
			RAM:    getProcessMemory(pe.ProcessID),
		}
		p.Path = imagePath(p.PID)
		pt.list = append(pt.list, p)
		pt.byPID[p.PID] = p
		if p.Path != "" {
			k := strings.ToLower(p.Path)
			pt.byPath[k] = append(pt.byPath[k], p.PID)
		}
		if windows.Process32Next(snap, &pe) != nil {
			break
		}
	}
	return pt
}

func visibleWindowPIDs() map[uint32]bool {
	out := map[uint32]bool{}
	cb := windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		if windows.IsWindowVisible(h) {
			var pid uint32
			windows.GetWindowThreadProcessId(h, &pid)
			out[pid] = true
		}
		return 1
	})
	windows.EnumWindows(cb, nil)
	return out
}

func imagePath(pid uint32) string {
	if pid == 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// descendantsOf returns all processes that descend from root (used to hide our own WebView2 helpers).
func (pt procTable) descendantsOf(root uint32) map[uint32]bool {
	out := map[uint32]bool{root: true}
	for changed := true; changed; {
		changed = false
		for _, p := range pt.list {
			if !out[p.PID] && out[p.PPID] {
				out[p.PID] = true
				changed = true
			}
		}
	}
	return out
}

// exePath pulls the executable out of a command line like `"C:\x\a.exe" -arg`.
func exePath(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	if exp, err := registry.ExpandString(cmd); err == nil {
		cmd = exp
	}
	if strings.HasPrefix(cmd, `"`) {
		if i := strings.Index(cmd[1:], `"`); i >= 0 {
			return cmd[1 : 1+i]
		}
		return ""
	}
	if i := strings.Index(strings.ToLower(cmd), ".exe"); i >= 0 {
		return cmd[:i+4]
	}
	return cmd
}

func (pt procTable) pidsFor(path string) []uint32 {
	if path == "" {
		return nil
	}
	return pt.byPath[strings.ToLower(path)]
}

func (pt procTable) ramFor(pids []uint32) uint64 {
	var total uint64
	seen := map[uint32]bool{}
	for _, pid := range pids {
		if !seen[pid] {
			seen[pid] = true
			if p, ok := pt.byPID[pid]; ok {
				total += p.RAM
			}
		}
	}
	return total
}
