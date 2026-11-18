package main

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type procInfo struct {
	PID, PPID  uint32
	Name, Path string
	Window     bool // owns at least one visible top-level window
}

type procTable struct {
	list   []procInfo
	byPath map[string][]uint32
	byPID  map[uint32]procInfo
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
		p := procInfo{PID: pe.ProcessID, PPID: pe.ParentProcessID,
			Name: windows.UTF16ToString(pe.ExeFile[:]), Window: wins[pe.ProcessID]}
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
