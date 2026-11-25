package backend

import (
	"testing"
	"unsafe"
)

func TestRestorePointStructSizes(t *testing.T) {
	if got := unsafe.Sizeof(restorePointInfoW{}); got != 528 {
		t.Fatalf("restorePointInfoW size = %d, want 528", got)
	}
	if got := unsafe.Sizeof(stateMgrStatus{}); got != 16 {
		t.Fatalf("stateMgrStatus size = %d, want 16", got)
	}
}

func TestGetConnectionsByPID(t *testing.T) {
	conns := GetConnectionsByPID()
	if conns == nil {
		t.Fatal("GetConnectionsByPID returned nil map")
	}
	t.Logf("Discovered active sockets across %d PIDs", len(conns))
}

func TestIsTelemetryHost(t *testing.T) {
	if !isTelemetryHost("telemetry.nvidia.com") {
		t.Error("expected telemetry.nvidia.com to be flagged")
	}
	if !isTelemetryHost("vortex.data.microsoft.com") {
		t.Error("expected vortex.data.microsoft.com to be flagged")
	}
	if isTelemetryHost("api.github.com") {
		t.Error("did not expect api.github.com to be flagged")
	}
}
