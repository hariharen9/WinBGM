package backend

import (
	"os"
	"testing"
)

func TestGetLiveProcessGroups(t *testing.T) {
	groups, tel := GetLiveProcessGroups()
	if len(groups) == 0 {
		t.Fatal("expected non-empty live process groups")
	}
	if tel.TotalPhysBytes == 0 {
		t.Error("expected TotalPhysBytes > 0")
	}
	t.Logf("Live groups: %d, Processes: %d, RAM: %s / %s (%d%%)",
		len(groups), tel.ProcessCount, tel.UsedPhysFmt, tel.TotalPhysFmt, tel.MemoryLoadPercent)
}

func TestFindLockingProcessesSelf(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	locks, err := FindLockingProcesses(exe)
	if err != nil {
		t.Fatalf("FindLockingProcesses failed: %v", err)
	}
	t.Logf("Found %d external locks on test binary", len(locks))
}
