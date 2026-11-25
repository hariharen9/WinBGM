package backend

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// DriveUsage summarizes capacity and usage for a fixed local volume.
type DriveUsage struct {
	Letter string `json:"letter"`
	Total  uint64 `json:"total"`
	Used   uint64 `json:"used"`
	Free   uint64 `json:"free"`
}

// SystemAboutInfo mirrors the Windows System > About hardware & OS specification page.
type SystemAboutInfo struct {
	// Top 4 highlight cards
	ProcessorName  string `json:"processorName"`
	ProcessorSpeed string `json:"processorSpeed"`
	LogicalCores   int    `json:"logicalCores"`
	RamSummary     string `json:"ramSummary"`
	RamSpeed       string `json:"ramSpeed"`
	GpuName        string `json:"gpuName"`
	GpuVram        string `json:"gpuVram"`
	StorageTotal   string `json:"storageTotal"`
	StorageUsed    string `json:"storageUsed"`

	// Device identity & board
	DeviceName       string       `json:"deviceName"`
	Motherboard      string       `json:"motherboard"`
	Manufacturer     string       `json:"manufacturer"`
	DeviceID         string       `json:"deviceId"`
	ProductID        string       `json:"productId"`
	SystemType       string       `json:"systemType"`
	Drives           []DriveUsage `json:"drives"`

	// Windows specification
	Edition     string `json:"edition"`
	Version     string `json:"version"`
	InstalledOn string `json:"installedOn"`
	OSBuild     string `json:"osBuild"`
	Experience  string `json:"experience"`
}

var (
	sysKernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procGetLogicalDriveStrings = sysKernel32.NewProc("GetLogicalDriveStringsW")
	procGetDriveTypeW          = sysKernel32.NewProc("GetDriveTypeW")
	procGetDiskFreeSpaceExW    = sysKernel32.NewProc("GetDiskFreeSpaceExW")
	procGetSystemFirmwareTable = sysKernel32.NewProc("GetSystemFirmwareTable")
)

// GetSystemAbout gathers hardware, motherboard, storage, and Windows OS details purely via Win32 APIs and Registry.
func GetSystemAbout() SystemAboutInfo {
	var info SystemAboutInfo
	info.LogicalCores = runtime.NumCPU()

	if host, err := os.Hostname(); err == nil && host != "" {
		info.DeviceName = host
	} else {
		info.DeviceName = os.Getenv("COMPUTERNAME")
	}

	readProcessorInfo(&info)
	readMemoryInfo(&info)
	readGPUInfo(&info)
	readStorageInfo(&info)
	readBIOSInfo(&info)
	readDeviceAndWindowsInfo(&info)

	return info
}

func readProcessorInfo(info *SystemAboutInfo) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()

	if name, _, err := k.GetStringValue("ProcessorNameString"); err == nil {
		info.ProcessorName = strings.TrimSpace(name)
	}
	if mhz, _, err := k.GetIntegerValue("~MHz"); err == nil && mhz > 0 {
		info.ProcessorSpeed = fmt.Sprintf("%.2f GHz", float64(mhz)/1000.0)
	}
}

func readMemoryInfo(info *SystemAboutInfo) {
	var ms memoryStatusEx
	ms.dwLength = uint32(unsafe.Sizeof(ms))
	r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms)))
	if r != 0 && ms.ullTotalPhys > 0 {
		gb := float64(ms.ullTotalPhys) / (1024 * 1024 * 1024)
		info.RamSummary = fmt.Sprintf("%.1f GB", gb)
	}

	if speed := readSMBIOSMemorySpeed(); speed > 0 {
		info.RamSpeed = fmt.Sprintf("Speed: %d MT/s", speed)
	}
}

// readSMBIOSMemorySpeed parses raw SMBIOS Type 17 (Memory Device) tables via GetSystemFirmwareTable('RSMB').
func readSMBIOSMemorySpeed() uint16 {
	// 'RSMB' in little-endian uint32: 0x52534D42
	const rsmb = 0x52534D42
	size, _, _ := procGetSystemFirmwareTable.Call(rsmb, 0, 0, 0)
	if size <= 8 || size > 1024*1024 {
		return 0
	}
	buf := make([]byte, size)
	ret, _, _ := procGetSystemFirmwareTable.Call(rsmb, 0, uintptr(unsafe.Pointer(&buf[0])), size)
	if ret < 8 {
		return 0
	}

	// RawSMBIOSData header is 8 bytes; table data starts at offset 8.
	data := buf[8:ret]
	i := 0
	var maxSpeed uint16
	for i+4 <= len(data) {
		stype := data[i]
		slen := int(data[i+1])
		if slen < 4 || i+slen > len(data) {
			break
		}
		if stype == 17 && slen >= 0x17 {
			// Offset 0x0C: Size (uint16). 0 means empty DIMM slot.
			dimmSize := binary.LittleEndian.Uint16(data[i+0x0C : i+0x0E])
			if dimmSize > 0 {
				var spd uint16
				if slen >= 0x22 {
					spd = binary.LittleEndian.Uint16(data[i+0x20 : i+0x22])
				}
				if spd == 0 {
					spd = binary.LittleEndian.Uint16(data[i+0x15 : i+0x17])
				}
				if spd > maxSpeed && spd != 0xFFFF {
					maxSpeed = spd
				}
			}
		}
		if stype == 127 {
			break
		}
		// Advance past formatted area and double-null-terminated string table.
		j := i + slen
		for j+1 < len(data) {
			if data[j] == 0 && data[j+1] == 0 {
				j += 2
				break
			}
			j++
		}
		i = j
	}
	return maxSpeed
}

func readGPUInfo(info *SystemAboutInfo) {
	const classKey = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, classKey, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return
	}
	defer root.Close()

	subs, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return
	}

	var fallbackName, fallbackVram string
	for _, sub := range subs {
		if len(sub) != 4 {
			continue
		}
		k, err := registry.OpenKey(root, sub, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		desc, _, err := k.GetStringValue("DriverDesc")
		var vramBytes uint64
		if qw, _, qerr := k.GetIntegerValue("HardwareInformation.qwMemorySize"); qerr == nil && qw > 0 {
			vramBytes = qw
		} else if dw, _, derr := k.GetIntegerValue("HardwareInformation.MemorySize"); derr == nil && dw > 0 {
			vramBytes = dw
		} else if b, _, berr := k.GetBinaryValue("HardwareInformation.MemorySize"); berr == nil && len(b) >= 4 {
			if len(b) >= 8 {
				vramBytes = binary.LittleEndian.Uint64(b[:8])
			} else {
				vramBytes = uint64(binary.LittleEndian.Uint32(b[:4]))
			}
		}
		k.Close()

		if err != nil || strings.TrimSpace(desc) == "" {
			continue
		}
		vramStr := formatVramBytes(vramBytes)
		lower := strings.ToLower(desc)
		isVirtual := strings.Contains(lower, "microsoft basic") || strings.Contains(lower, "virtual") || strings.Contains(lower, "remote")
		if !isVirtual && vramBytes > 0 {
			info.GpuName = strings.TrimSpace(desc)
			info.GpuVram = vramStr
			return
		}
		if fallbackName == "" {
			fallbackName = strings.TrimSpace(desc)
			fallbackVram = vramStr
		}
	}

	if info.GpuName == "" {
		info.GpuName = fallbackName
		info.GpuVram = fallbackVram
	}
}

func formatVramBytes(b uint64) string {
	if b == 0 {
		return ""
	}
	gb := float64(b) / (1024 * 1024 * 1024)
	if gb >= 1.0 {
		if gb == float64(int(gb)) || (gb-float64(int(gb))) < 0.05 {
			return fmt.Sprintf("%d GB", int(gb+0.5))
		}
		return fmt.Sprintf("%.1f GB", gb)
	}
	mb := float64(b) / (1024 * 1024)
	return fmt.Sprintf("%.0f MB", mb)
}

func readStorageInfo(info *SystemAboutInfo) {
	buf := make([]uint16, 256)
	n, _, _ := procGetLogicalDriveStrings.Call(uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	if n == 0 {
		return
	}

	var totalAll, freeAll uint64
	i := 0
	for i < int(n) {
		start := i
		for i < int(n) && buf[i] != 0 {
			i++
		}
		if i > start {
			rootPtr := &buf[start]
			dtype, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(rootPtr)))
			if dtype == 3 { // DRIVE_FIXED
				var freeAvail, totalBytes, totalFree uint64
				r, _, _ := procGetDiskFreeSpaceExW.Call(
					uintptr(unsafe.Pointer(rootPtr)),
					uintptr(unsafe.Pointer(&freeAvail)),
					uintptr(unsafe.Pointer(&totalBytes)),
					uintptr(unsafe.Pointer(&totalFree)),
				)
				if r != 0 && totalBytes > 0 {
					used := totalBytes - totalFree
					totalAll += totalBytes
					freeAll += totalFree
					letter := strings.TrimSuffix(syscall.UTF16ToString(buf[start:i]), `\`)
					info.Drives = append(info.Drives, DriveUsage{
						Letter: letter,
						Total:  totalBytes,
						Used:   used,
						Free:   totalFree,
					})
				}
			}
		}
		i++
	}

	if totalAll > 0 {
		usedAll := totalAll - freeAll
		info.StorageTotal = formatStorageSize(totalAll)
		info.StorageUsed = fmt.Sprintf("%s of %s used", formatStorageSize(usedAll), info.StorageTotal)
	}
}

func formatStorageSize(b uint64) string {
	const tb = 1024 * 1024 * 1024 * 1024
	const gb = 1024 * 1024 * 1024
	if b >= tb {
		return fmt.Sprintf("%.2f TB", float64(b)/float64(tb))
	}
	return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
}

func readBIOSInfo(info *SystemAboutInfo) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()

	if prod, _, err := k.GetStringValue("BaseBoardProduct"); err == nil && strings.TrimSpace(prod) != "" {
		info.Motherboard = strings.TrimSpace(prod)
	} else if sysProd, _, err := k.GetStringValue("SystemProductName"); err == nil {
		info.Motherboard = strings.TrimSpace(sysProd)
	}

	if mfg, _, err := k.GetStringValue("BaseBoardManufacturer"); err == nil && strings.TrimSpace(mfg) != "" {
		info.Manufacturer = strings.TrimSpace(mfg)
	} else if sysMfg, _, err := k.GetStringValue("SystemManufacturer"); err == nil {
		info.Manufacturer = strings.TrimSpace(sysMfg)
	}
}

func readDeviceAndWindowsInfo(info *SystemAboutInfo) {
	if runtime.GOARCH == "amd64" {
		info.SystemType = "64-bit operating system, x64-based processor"
	} else if runtime.GOARCH == "arm64" {
		info.SystemType = "64-bit operating system, ARM-based processor"
	} else {
		info.SystemType = "32-bit operating system, x86-based processor"
	}

	if sqm, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\SQMClient`, registry.QUERY_VALUE); err == nil {
		if id, _, err := sqm.GetStringValue("MachineId"); err == nil {
			info.DeviceID = strings.Trim(strings.TrimSpace(id), "{}")
		}
		sqm.Close()
	}

	nt, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer nt.Close()

	if pid, _, err := nt.GetStringValue("ProductId"); err == nil {
		info.ProductID = strings.TrimSpace(pid)
	}
	if ver, _, err := nt.GetStringValue("DisplayVersion"); err == nil && ver != "" {
		info.Version = strings.TrimSpace(ver)
	} else if rel, _, err := nt.GetStringValue("ReleaseId"); err == nil {
		info.Version = strings.TrimSpace(rel)
	}

	buildStr, _, _ := nt.GetStringValue("CurrentBuildNumber")
	buildNum, _ := strconv.Atoi(strings.TrimSpace(buildStr))
	if ubr, _, err := nt.GetIntegerValue("UBR"); err == nil {
		info.OSBuild = fmt.Sprintf("%s.%d", strings.TrimSpace(buildStr), ubr)
	} else {
		info.OSBuild = strings.TrimSpace(buildStr)
	}

	if prod, _, err := nt.GetStringValue("ProductName"); err == nil {
		edition := strings.TrimSpace(prod)
		if buildNum >= 22000 {
			edition = strings.Replace(edition, "Windows 10", "Windows 11", 1)
		}
		info.Edition = edition
	}

	if inst, _, err := nt.GetIntegerValue("InstallDate"); err == nil && inst > 0 {
		info.InstalledOn = time.Unix(int64(inst), 0).Format("02-01-2006")
	}

	info.Experience = readFeatureExperiencePack()
}

func readFeatureExperiencePack() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Appx\AppxAllUserStore\InboxApplications`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return ""
	}
	defer k.Close()

	subs, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return ""
	}
	for _, sub := range subs {
		if strings.HasPrefix(sub, "MicrosoftWindows.Client.CBS_") {
			parts := strings.Split(sub, "_")
			if len(parts) >= 2 {
				return "Windows Feature Experience Pack " + parts[1]
			}
		}
	}
	return ""
}
