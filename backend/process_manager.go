package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modKernel32PM              = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes         = modKernel32PM.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx   = modKernel32PM.NewProc("GlobalMemoryStatusEx")
	procGetProcessIoCounters   = modKernel32PM.NewProc("GetProcessIoCounters")
	procGetPriorityClass       = modKernel32PM.NewProc("GetPriorityClass")
	procSetPriorityClass       = modKernel32PM.NewProc("SetPriorityClass")
	procSetProcessInformation  = modKernel32PM.NewProc("SetProcessInformation")
	procGetProcessInformation  = modKernel32PM.NewProc("GetProcessInformation")
)

const (
	processPowerThrottling               = 4
	processPowerThrottlingVersion        = 1
	processPowerThrottlingExecutionSpeed = 0x1

	idlePriorityClass        = 0x00000040
	belowNormalPriorityClass = 0x00004000
	normalPriorityClass      = 0x00000020
	aboveNormalPriorityClass = 0x00008000
	highPriorityClass        = 0x00000080
)

type memoryStatusEx struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type processPowerThrottlingState struct {
	Version     uint32
	ControlMask uint32
	StateMask   uint32
}

// SystemTelemetry holds real-time machine-wide CPU, RAM, and socket metrics.
type SystemTelemetry struct {
	CPUPercent        float64 `json:"cpuPercent"`
	TotalPhysBytes    uint64  `json:"totalPhysBytes"`
	UsedPhysBytes     uint64  `json:"usedPhysBytes"`
	TotalPhysFmt      string  `json:"totalPhysFormatted"`
	UsedPhysFmt       string  `json:"usedPhysFormatted"`
	MemoryLoadPercent uint32  `json:"memoryLoadPercent"`
	TotalSockets      int     `json:"totalSockets"`
	TelemetrySockets  int     `json:"telemetrySockets"`
	ProcessCount      int     `json:"processCount"`
}

// ProcessEntry represents an individual running OS process.
type ProcessEntry struct {
	PID           uint32  `json:"pid"`
	PPID          uint32  `json:"ppid"`
	Name          string  `json:"name"`
	ExePath       string  `json:"exePath"`
	CPUPercent    float64 `json:"cpuPercent"`
	RAMBytes      uint64  `json:"ramBytes"`
	RAMFormatted  string  `json:"ramFormatted"`
	IOBytesPerSec uint64  `json:"ioBytesPerSec"`
	IOFormatted   string  `json:"ioFormatted"`
	Priority      string  `json:"priority"`
	EcoMode       bool    `json:"ecoMode"`
	Suspended     bool    `json:"suspended"`
	HasWindow     bool    `json:"hasWindow"`
	Protected     bool    `json:"protected"`
	ConnCount     int     `json:"connCount"`
}

// ProcessGroup aggregates multiple instances of the same executable (like browser tabs).
type ProcessGroup struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	ExePath        string         `json:"exePath"`
	PIDs           []uint32       `json:"pids"`
	ProcessCount   int            `json:"processCount"`
	CPUPercent     float64        `json:"cpuPercent"`
	RAMBytes       uint64         `json:"ramBytes"`
	RAMFormatted   string         `json:"ramFormatted"`
	IOBytesPerSec  uint64         `json:"ioBytesPerSec"`
	IOFormatted    string         `json:"ioFormatted"`
	Priority       string         `json:"priority"`
	EcoMode        bool           `json:"ecoMode"`
	Suspended      bool           `json:"suspended"`
	HasWindow      bool           `json:"hasWindow"`
	Protected      bool           `json:"protected"`
	ConnCount      int            `json:"connCount"`
	TelemetryCount int            `json:"telemetryCount"`
	Children       []ProcessEntry `json:"children"`
}

var (
	pmMu              sync.Mutex
	prevSysIdle       uint64
	prevSysKernel     uint64
	prevSysUser       uint64
	prevGroupSysTotal uint64
	prevProcCPU       = make(map[uint32]uint64)
	prevProcIO        = make(map[uint32]uint64)
	prevSampleTime    time.Time
	ecoModeMap        = make(map[uint32]bool)
)

var protectedProcessNames = map[string]bool{
	"system":                    true,
	"idle":                      true,
	"registry":                  true,
	"smss.exe":                  true,
	"csrss.exe":                 true,
	"wininit.exe":               true,
	"services.exe":              true,
	"svchost.exe":               true,
	"lsass.exe":                 true,
	"lsm.exe":                   true,
	"winlogon.exe":              true,
	"dwm.exe":                   true,
	"fontdrvhost.exe":           true,
	"memory compression":        true,
	"msmpeng.exe":               true,
	"nissrv.exe":                true,
	"securityhealthservice.exe": true,
	"winbgm.exe":                true,
}

func ftToUint64(ft windows.Filetime) uint64 {
	return (uint64(ft.HighDateTime) << 32) | uint64(ft.LowDateTime)
}

func isProtectedProcess(name string, pid uint32) bool {
	if pid == 0 || pid == 4 || pid == uint32(os.Getpid()) {
		return true
	}
	return protectedProcessNames[strings.ToLower(name)]
}

func priorityStr(cls uint32) string {
	switch cls {
	case idlePriorityClass:
		return "Idle"
	case belowNormalPriorityClass:
		return "Below Normal"
	case normalPriorityClass:
		return "Normal"
	case aboveNormalPriorityClass:
		return "Above Normal"
	case highPriorityClass:
		return "High"
	case 0x00000100:
		return "Realtime"
	default:
		return "Normal"
	}
}

func querySystemTelemetryLocked() (SystemTelemetry, uint64) {
	var st SystemTelemetry

	// Physical RAM
	var mem memoryStatusEx
	mem.dwLength = uint32(unsafe.Sizeof(mem))
	if ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem))); ret != 0 {
		st.TotalPhysBytes = mem.ullTotalPhys
		st.UsedPhysBytes = mem.ullTotalPhys - mem.ullAvailPhys
		st.TotalPhysFmt = FormatBytes(st.TotalPhysBytes)
		st.UsedPhysFmt = FormatBytes(st.UsedPhysBytes)
		st.MemoryLoadPercent = mem.dwMemoryLoad
	}

	// System CPU %
	var idleFT, kernelFT, userFT windows.Filetime
	var sysDelta uint64
	if ret, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleFT)),
		uintptr(unsafe.Pointer(&kernelFT)),
		uintptr(unsafe.Pointer(&userFT)),
	); ret != 0 {
		idle := ftToUint64(idleFT)
		kernel := ftToUint64(kernelFT)
		user := ftToUint64(userFT)

		if prevSysKernel != 0 || prevSysUser != 0 {
			idleDelta := idle - prevSysIdle
			kernelDelta := kernel - prevSysKernel
			userDelta := user - prevSysUser
			sysDelta = kernelDelta + userDelta
			if sysDelta > 0 && sysDelta >= idleDelta {
				st.CPUPercent = float64(sysDelta-idleDelta) / float64(sysDelta) * 100.0
				if st.CPUPercent < 0 {
					st.CPUPercent = 0
				} else if st.CPUPercent > 100 {
					st.CPUPercent = 100
				}
			}
		}
		prevSysIdle = idle
		prevSysKernel = kernel
		prevSysUser = user
	}

	return st, sysDelta
}

func getProcCPUTimeAndIO(pid uint32) (cpuTime uint64, ioBytes uint64, priority string, eco bool) {
	priority = "Normal"
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, 0, priority, ecoModeMap[pid]
	}
	defer windows.CloseHandle(h)

	var cTime, eTime, kTime, uTime windows.Filetime
	if err := windows.GetProcessTimes(h, &cTime, &eTime, &kTime, &uTime); err == nil {
		cpuTime = ftToUint64(kTime) + ftToUint64(uTime)
	}

	var ioc ioCounters
	if ret, _, _ := procGetProcessIoCounters.Call(uintptr(h), uintptr(unsafe.Pointer(&ioc))); ret != 0 {
		ioBytes = ioc.ReadTransferCount + ioc.WriteTransferCount
	}

	if cls, _, _ := procGetPriorityClass.Call(uintptr(h)); cls != 0 {
		priority = priorityStr(uint32(cls))
	}

	var ppts processPowerThrottlingState
	ppts.Version = processPowerThrottlingVersion
	if ret, _, _ := procGetProcessInformation.Call(
		uintptr(h),
		uintptr(processPowerThrottling),
		uintptr(unsafe.Pointer(&ppts)),
		unsafe.Sizeof(ppts),
	); ret != 0 {
		eco = (ppts.StateMask & processPowerThrottlingExecutionSpeed) != 0
		ecoModeMap[pid] = eco
	} else {
		eco = ecoModeMap[pid]
	}

	return cpuTime, ioBytes, priority, eco
}

// GetLiveProcessGroups returns all running processes grouped by application with CPU%, RAM, I/O, EcoQoS, and sockets.
func GetLiveProcessGroups() ([]ProcessGroup, SystemTelemetry) {
	pt := snapshotProcs()
	conns := GetConnectionsByPID()

	pmMu.Lock()
	defer pmMu.Unlock()

	now := time.Now()
	elapsedSec := now.Sub(prevSampleTime).Seconds()
	if elapsedSec <= 0.1 || prevSampleTime.IsZero() {
		elapsedSec = 3.0
	}

	// Read system telemetry without disturbing prevGroupSysTotal, and compute dedicated group sysDelta
	var sysTel SystemTelemetry
	var mem memoryStatusEx
	mem.dwLength = uint32(unsafe.Sizeof(mem))
	if ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem))); ret != 0 {
		sysTel.TotalPhysBytes = mem.ullTotalPhys
		sysTel.UsedPhysBytes = mem.ullTotalPhys - mem.ullAvailPhys
		sysTel.TotalPhysFmt = FormatBytes(sysTel.TotalPhysBytes)
		sysTel.UsedPhysFmt = FormatBytes(sysTel.UsedPhysBytes)
		sysTel.MemoryLoadPercent = mem.dwMemoryLoad
	}

	var sysDelta uint64
	var idleFT, kernelFT, userFT windows.Filetime
	if ret, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleFT)),
		uintptr(unsafe.Pointer(&kernelFT)),
		uintptr(unsafe.Pointer(&userFT)),
	); ret != 0 {
		idle := ftToUint64(idleFT)
		kernel := ftToUint64(kernelFT)
		user := ftToUint64(userFT)
		curTotal := kernel + user

		if prevGroupSysTotal != 0 && curTotal > prevGroupSysTotal {
			sysDelta = curTotal - prevGroupSysTotal
		}
		prevGroupSysTotal = curTotal

		if prevSysKernel != 0 || prevSysUser != 0 {
			telDelta := (kernel - prevSysKernel) + (user - prevSysUser)
			idleDelta := idle - prevSysIdle
			if telDelta > 0 && telDelta >= idleDelta {
				sysTel.CPUPercent = float64(telDelta-idleDelta) / float64(telDelta) * 100.0
				if sysTel.CPUPercent > 100 {
					sysTel.CPUPercent = 100
				}
			}
		}
	}
	sysTel.ProcessCount = len(pt.list)

	totalSockets := 0
	telemetrySockets := 0
	for _, list := range conns {
		totalSockets += len(list)
		for _, c := range list {
			if c.IsTelemetry {
				telemetrySockets++
			}
		}
	}
	sysTel.TotalSockets = totalSockets
	sysTel.TelemetrySockets = telemetrySockets

	seenPIDs := make(map[uint32]bool, len(pt.list))
	groupsMap := make(map[string]*ProcessGroup)

	for _, p := range pt.list {
		seenPIDs[p.PID] = true
		cpuTime, ioBytes, prio, eco := getProcCPUTimeAndIO(p.PID)

		var cpuPct float64
		if prevT, ok := prevProcCPU[p.PID]; ok && sysDelta > 0 && cpuTime >= prevT {
			cpuPct = float64(cpuTime-prevT) / float64(sysDelta) * 100.0
			if cpuPct > 100 {
				cpuPct = 100
			}
		}
		prevProcCPU[p.PID] = cpuTime

		var ioRate uint64
		if prevIO, ok := prevProcIO[p.PID]; ok && ioBytes >= prevIO && elapsedSec > 0 {
			ioRate = uint64(float64(ioBytes-prevIO) / elapsedSec)
		}
		prevProcIO[p.PID] = ioBytes

		pConns := conns[p.PID]
		pTelCount := 0
		for _, c := range pConns {
			if c.IsTelemetry {
				pTelCount++
			}
		}

		entry := ProcessEntry{
			PID:           p.PID,
			PPID:          p.PPID,
			Name:          p.Name,
			ExePath:       p.Path,
			CPUPercent:    cpuPct,
			RAMBytes:      p.RAM,
			RAMFormatted:  formatRAM(p.RAM),
			IOBytesPerSec: ioRate,
			IOFormatted:   formatIORate(ioRate),
			Priority:      prio,
			EcoMode:       eco,
			Suspended:     IsProcessSuspended(p.PID),
			HasWindow:     p.Window,
			Protected:     isProtectedProcess(p.Name, p.PID),
			ConnCount:     len(pConns),
		}

		key := strings.ToLower(p.Path)
		if key == "" {
			key = "name:" + strings.ToLower(p.Name)
		}

		grp, exists := groupsMap[key]
		if !exists {
			displayName := strings.TrimSuffix(p.Name, filepath.Ext(p.Name))
			if displayName == "" {
				displayName = p.Name
			}
			grp = &ProcessGroup{
				ID:        key,
				Name:      displayName,
				ExePath:   p.Path,
				Priority:  prio,
				Protected: entry.Protected,
			}
			groupsMap[key] = grp
		}

		grp.PIDs = append(grp.PIDs, p.PID)
		grp.ProcessCount++
		grp.CPUPercent += cpuPct
		grp.RAMBytes += p.RAM
		grp.IOBytesPerSec += ioRate
		grp.ConnCount += len(pConns)
		grp.TelemetryCount += pTelCount
		if p.Window {
			grp.HasWindow = true
		}
		if eco {
			grp.EcoMode = true
		}
		if entry.Suspended {
			grp.Suspended = true
		}
		if entry.Protected {
			grp.Protected = true
		}
		grp.Children = append(grp.Children, entry)
	}

	// Clean up exited PIDs from maps
	for pid := range prevProcCPU {
		if !seenPIDs[pid] {
			delete(prevProcCPU, pid)
			delete(prevProcIO, pid)
			delete(ecoModeMap, pid)
		}
	}
	prevSampleTime = now

	groups := make([]ProcessGroup, 0, len(groupsMap))
	for _, grp := range groupsMap {
		if grp.CPUPercent > 100.0 {
			grp.CPUPercent = 100.0
		}
		grp.RAMFormatted = formatRAM(grp.RAMBytes)
		grp.IOFormatted = formatIORate(grp.IOBytesPerSec)
		sort.Slice(grp.Children, func(i, j int) bool {
			return grp.Children[i].RAMBytes > grp.Children[j].RAMBytes
		})
		groups = append(groups, *grp)
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].RAMBytes != groups[j].RAMBytes {
			return groups[i].RAMBytes > groups[j].RAMBytes
		}
		return strings.ToLower(groups[i].Name) < strings.ToLower(groups[j].Name)
	})

	return groups, sysTel
}

func formatIORate(bytesPerSec uint64) string {
	if bytesPerSec < 1024 {
		return "0 KB/s"
	}
	if bytesPerSec < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", float64(bytesPerSec)/1024.0)
	}
	return fmt.Sprintf("%.1f MB/s", float64(bytesPerSec)/(1024.0*1024.0))
}

// SetProcessEcoMode enables or disables Windows 11 Efficiency Mode (EcoQoS) on a process.
func SetProcessEcoMode(pid uint32, enable bool) error {
	if pid == 0 || pid == 4 || pid == uint32(os.Getpid()) {
		return fmt.Errorf("cannot modify protected system or self process")
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("failed to open process %d (try running as administrator): %w", pid, err)
	}
	defer windows.CloseHandle(h)

	var stateMask uint32
	if enable {
		stateMask = processPowerThrottlingExecutionSpeed
	}

	state := processPowerThrottlingState{
		Version:     processPowerThrottlingVersion,
		ControlMask: processPowerThrottlingExecutionSpeed,
		StateMask:   stateMask,
	}

	ret, _, callErr := procSetProcessInformation.Call(
		uintptr(h),
		uintptr(processPowerThrottling),
		uintptr(unsafe.Pointer(&state)),
		unsafe.Sizeof(state),
	)
	if ret == 0 {
		return fmt.Errorf("SetProcessInformation failed (Windows 10 21H2+ / Windows 11 required): %v", callErr)
	}

	pmMu.Lock()
	ecoModeMap[pid] = enable
	pmMu.Unlock()
	return nil
}

// SetProcessPriority changes the scheduling priority class of a process.
func SetProcessPriority(pid uint32, level string) error {
	if pid == 0 || pid == 4 || pid == uint32(os.Getpid()) {
		return fmt.Errorf("cannot modify protected system or self process")
	}
	var cls uint32
	switch strings.ToLower(level) {
	case "idle", "low":
		cls = idlePriorityClass
	case "below normal", "belownormal":
		cls = belowNormalPriorityClass
	case "normal":
		cls = normalPriorityClass
	case "above normal", "abovenormal":
		cls = aboveNormalPriorityClass
	case "high":
		cls = highPriorityClass
	default:
		return fmt.Errorf("unsupported priority level: %s", level)
	}

	h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("failed to open process %d (try running as administrator): %w", pid, err)
	}
	defer windows.CloseHandle(h)

	ret, _, callErr := procSetPriorityClass.Call(uintptr(h), uintptr(cls))
	if ret == 0 {
		return fmt.Errorf("SetPriorityClass failed: %v", callErr)
	}
	return nil
}
