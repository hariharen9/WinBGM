package backend

import "testing"

func TestGetSystemAbout(t *testing.T) {
	info := GetSystemAbout()
	if info.DeviceName == "" {
		t.Fatal("expected non-empty DeviceName")
	}
	if info.ProcessorName == "" {
		t.Fatal("expected non-empty ProcessorName")
	}
	if info.RamSummary == "" {
		t.Fatal("expected non-empty RamSummary")
	}
	if info.OSBuild == "" {
		t.Fatal("expected non-empty OSBuild")
	}
	t.Logf("SystemAboutInfo: %+v", info)
}
