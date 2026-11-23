package backend

import (
	"os"
	"testing"
)

func TestPollProcesses(t *testing.T) {
	res := PollProcesses()

	if len(res.RunningPIDs) == 0 {
		t.Fatal("Expected running PIDs, got 0")
	}
	if res.TotalRAMBytes == 0 {
		t.Fatal("Expected total RAM bytes > 0")
	}
	if res.TotalRAMFormatted == "" {
		t.Fatal("Expected formatted RAM string, got empty")
	}

	t.Logf("Polled %d running PIDs, Total RAM: %s (%d bytes)",
		len(res.RunningPIDs), res.TotalRAMFormatted, res.TotalRAMBytes)

	// Current process should be in the snapshot
	myPID := uint32(os.Getpid())
	foundSelf := false
	for _, pid := range res.RunningPIDs {
		if pid == myPID {
			foundSelf = true
			break
		}
	}
	if !foundSelf {
		t.Errorf("Current test process PID %d was not found in running PIDs", myPID)
	}

	// Active exe paths map should not be empty
	if len(res.ActiveExePaths) == 0 {
		t.Error("Expected ActiveExePaths map to have entries")
	}
	t.Logf("Unique active exe paths tracked: %d", len(res.ActiveExePaths))
}
