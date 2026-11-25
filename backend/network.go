package backend

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modIphlpapi             = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = modIphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = modIphlpapi.NewProc("GetExtendedUdpTable")
)

const (
	afInet              = 2
	tcpTableOwnerPidAll = 5
	udpTableOwnerPidAll = 1
)

type mibTcpRowOwnerPid struct {
	dwState      uint32
	dwLocalAddr  uint32
	dwLocalPort  uint32
	dwRemoteAddr uint32
	dwRemotePort uint32
	dwOwningPid  uint32
}

type mibUdpRowOwnerPid struct {
	dwLocalAddr uint32
	dwLocalPort uint32
	dwOwningPid uint32
}

// NetworkConnection represents an active TCP or UDP socket owned by a running process.
type NetworkConnection struct {
	PID         uint32 `json:"pid"`
	Protocol    string `json:"protocol"`
	LocalAddr   string `json:"localAddr"`
	LocalPort   uint16 `json:"localPort"`
	RemoteAddr  string `json:"remoteAddr"`
	RemotePort  uint16 `json:"remotePort"`
	RemoteHost  string `json:"remoteHost"`
	State       string `json:"state"`
	IsTelemetry bool   `json:"isTelemetry"`
}

var (
	dnsMu    sync.RWMutex
	dnsCache = make(map[string]string)
	dnsInFly = make(map[string]bool)
)

var telemetryKeywords = []string{
	"telemetry", "metrics", "analytics", "tracking", "diagnostic",
	"beacon", "collector", "crash", "sentry", "mixpanel", "amplitude",
	"segment.", "vortex.data", "watson.", "data.microsoft", "app-measurement",
	"bugsnag", "datadog", "newrelic", "appsflyer",
}

func isTelemetryHost(host string) bool {
	if host == "" {
		return false
	}
	low := strings.ToLower(host)
	for _, kw := range telemetryKeywords {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

func resolveHostAsync(ipStr string) string {
	if ipStr == "" || ipStr == "0.0.0.0" || ipStr == "127.0.0.1" || ipStr == "::" || ipStr == "::1" {
		return ""
	}
	// Skip private RFC1918 ranges from PTR lookups
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsMulticast() {
		return ""
	}

	dnsMu.RLock()
	cached, ok := dnsCache[ipStr]
	inFly := dnsInFly[ipStr]
	dnsMu.RUnlock()

	if ok {
		return cached
	}
	if inFly {
		return ""
	}

	dnsMu.Lock()
	if !dnsInFly[ipStr] {
		dnsInFly[ipStr] = true
		go func(targetIP string) {
			ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			defer cancel()
			names, err := net.DefaultResolver.LookupAddr(ctx, targetIP)
			resolved := ""
			if err == nil && len(names) > 0 {
				resolved = strings.TrimSuffix(names[0], ".")
			}
			dnsMu.Lock()
			dnsCache[targetIP] = resolved
			delete(dnsInFly, targetIP)
			dnsMu.Unlock()
		}(ipStr)
	}
	dnsMu.Unlock()
	return ""
}

func ntohs(p uint32) uint16 {
	return uint16((p&0xff)<<8 | (p>>8)&0xff)
}

func ipv4Str(addr uint32) string {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], addr)
	return net.IPv4(b[0], b[1], b[2], b[3]).String()
}

func tcpStateStr(s uint32) string {
	switch s {
	case 1:
		return "CLOSED"
	case 2:
		return "LISTEN"
	case 3:
		return "SYN_SENT"
	case 4:
		return "SYN_RCVD"
	case 5:
		return "ESTABLISHED"
	case 6:
		return "FIN_WAIT1"
	case 7:
		return "FIN_WAIT2"
	case 8:
		return "CLOSE_WAIT"
	case 9:
		return "CLOSING"
	case 10:
		return "LAST_ACK"
	case 11:
		return "TIME_WAIT"
	case 12:
		return "DELETE_TCB"
	default:
		return "UNKNOWN"
	}
}

// GetConnectionsByPID queries iphlpapi.dll for all active IPv4 TCP and UDP sockets grouped by owning PID.
func GetConnectionsByPID() map[uint32][]NetworkConnection {
	out := make(map[uint32][]NetworkConnection)

	// 1. TCP Table (IPv4)
	if err := procGetExtendedTcpTable.Find(); err == nil {
		var size uint32
		procGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, afInet, tcpTableOwnerPidAll, 0)
		if size > 0 {
			buf := make([]byte, size)
			ret, _, _ := procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, afInet, tcpTableOwnerPidAll, 0)
			if ret == 0 && len(buf) >= 4 {
				numEntries := binary.LittleEndian.Uint32(buf[:4])
				rowSize := unsafe.Sizeof(mibTcpRowOwnerPid{})
				for i := uint32(0); i < numEntries; i++ {
					offset := 4 + uintptr(i)*rowSize
					if offset+rowSize > uintptr(len(buf)) {
						break
					}
					row := (*mibTcpRowOwnerPid)(unsafe.Pointer(&buf[offset]))
					if row.dwOwningPid == 0 {
						continue
					}
					remoteIP := ipv4Str(row.dwRemoteAddr)
					remotePort := ntohs(row.dwRemotePort)
					state := tcpStateStr(row.dwState)
					// Skip idle/closed local-only sockets unless established or listening
					if state == "TIME_WAIT" || state == "CLOSED" {
						continue
					}
					host := ""
					if remoteIP != "0.0.0.0" {
						host = resolveHostAsync(remoteIP)
					} else {
						remotePort = 0
					}
					out[row.dwOwningPid] = append(out[row.dwOwningPid], NetworkConnection{
						PID:         row.dwOwningPid,
						Protocol:    "TCP",
						LocalAddr:   ipv4Str(row.dwLocalAddr),
						LocalPort:   ntohs(row.dwLocalPort),
						RemoteAddr:  remoteIP,
						RemotePort:  remotePort,
						RemoteHost:  host,
						State:       state,
						IsTelemetry: isTelemetryHost(host),
					})
				}
			}
		}
	}

	// 2. UDP Table (IPv4)
	if err := procGetExtendedUdpTable.Find(); err == nil {
		var size uint32
		procGetExtendedUdpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, afInet, udpTableOwnerPidAll, 0)
		if size > 0 {
			buf := make([]byte, size)
			ret, _, _ := procGetExtendedUdpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, afInet, udpTableOwnerPidAll, 0)
			if ret == 0 && len(buf) >= 4 {
				numEntries := binary.LittleEndian.Uint32(buf[:4])
				rowSize := unsafe.Sizeof(mibUdpRowOwnerPid{})
				for i := uint32(0); i < numEntries; i++ {
					offset := 4 + uintptr(i)*rowSize
					if offset+rowSize > uintptr(len(buf)) {
						break
					}
					row := (*mibUdpRowOwnerPid)(unsafe.Pointer(&buf[offset]))
					if row.dwOwningPid == 0 {
						continue
					}
					out[row.dwOwningPid] = append(out[row.dwOwningPid], NetworkConnection{
						PID:        row.dwOwningPid,
						Protocol:   "UDP",
						LocalAddr:  ipv4Str(row.dwLocalAddr),
						LocalPort:  ntohs(row.dwLocalPort),
						RemoteAddr: "*",
						State:      "OPEN",
					})
				}
			}
		}
	}

	return out
}
