package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyAndRestoreSafetyRejections(t *testing.T) {
	// Nil inventory
	if res := Apply(nil, ChangeRequest{ID: "x", Action: "disable"}); res.OK {
		t.Error("expected Apply(nil, ...) to fail")
	}

	// Protected Windows component
	inv := &Inventory{
		Apps: []AppGroup{
			{
				ID:   "app1",
				Name: "Windows Defender",
				Components: []Component{
					{
						ID:      "comp_prot",
						Kind:    "service",
						Name:    "WinDefend",
						System:  true,
						Control: "no",
						ExePath: `C:\Windows\System32\MsMpEng.exe`,
					},
					{
						ID:      "comp_run",
						Kind:    "run",
						Name:    "TestApp",
						Control: "yes",
						Enabled: true,
					},
				},
			},
		},
	}

	if res := Apply(inv, ChangeRequest{ID: "missing", Action: "disable"}); res.OK {
		t.Error("expected Apply with unknown ID to fail")
	}
	if res := Apply(inv, ChangeRequest{ID: "comp_prot", Action: "disable"}); res.OK {
		t.Error("expected Apply on protected Windows/AV service to be refused")
	}
	if res := Apply(inv, ChangeRequest{ID: "comp_run", Action: "stop"}); res.OK {
		t.Error("expected Apply('stop') on non-service run entry to fail")
	}

	// Restore with unknown ID must fail safely
	if res := Restore("nonexistent-history-id"); res.OK {
		t.Error("expected Restore of nonexistent ID to fail")
	}
}

func TestCleanerPathAndRegistryValidation(t *testing.T) {
	badPaths := []string{
		"",
		`C:\`,
		`C:\Windows`,
		`C:\Windows\System32`,
		os.Getenv("USERPROFILE"),
		os.Getenv("APPDATA"),
		os.Getenv("LOCALAPPDATA"),
		os.Getenv("ProgramData"),
		os.Getenv("ProgramFiles"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft"),
		filepath.Join(os.Getenv("APPDATA"), "Google"),
	}
	for _, p := range badPaths {
		if p == "" {
			continue
		}
		if err := ValidateCleanPath(p); err == nil {
			t.Errorf("ValidateCleanPath(%q) should have failed, but passed", p)
		}
	}

	goodPath := filepath.Join(os.Getenv("LOCALAPPDATA"), "WinBGMTestTempApp")
	if err := ValidateCleanPath(goodPath); err != nil {
		t.Errorf("ValidateCleanPath(%q) unexpectedly failed: %v", goodPath, err)
	}

	badKeys := []string{
		`HKCU\Software`,
		`HKCU\Software\Microsoft`,
		`HKCU\Software\Microsoft\Windows`,
		`HKLM\Software\Google`,
		`HKLM\SYSTEM\CurrentControlSet`,
	}
	for _, k := range badKeys {
		if _, _, err := ValidateCleanRegistryPath(k); err == nil {
			t.Errorf("ValidateCleanRegistryPath(%q) should have failed, but passed", k)
		}
	}
}

func TestCandidateAppSubpathsRejectsBroadTokens(t *testing.T) {
	cases := []InstalledApp{
		{Name: "Microsoft Edge", Publisher: "Microsoft Corporation"},
		{Name: "Google Chrome", Publisher: "Google LLC"},
		{Name: "Windows App", Publisher: "Microsoft"},
	}
	for _, app := range cases {
		subs := candidateAppSubpaths(app)
		for _, s := range subs {
			low := strings.ToLower(s)
			if low == "microsoft" || low == "google" || low == "windows" {
				t.Errorf("candidateAppSubpaths(%q) produced dangerous broad token %q", app.Name, s)
			}
		}
	}
}

func TestScanResidualsNeverPreSelectsAndRejectsForgedIDs(t *testing.T) {
	report, err := ScanResiduals(InstalledApp{ID: "HKCU_Ollama", Name: "Ollama"})
	if err != nil {
		t.Fatalf("ScanResiduals failed: %v", err)
	}
	for _, it := range report.Items {
		if it.IsSelected {
			t.Errorf("ResidualItem %q (%s) was pre-selected; expected IsSelected=false", it.ID, it.Path)
		}
	}

	// Forged cleanup request with uncached ID & dangerous path must be rejected
	res, err := CleanResidualItems([]ResidualItem{
		{
			ID:         "forged_id_999",
			Path:       `C:\Windows\System32`,
			IsSelected: true,
			IsRegistry: false,
		},
	})
	if err != nil {
		t.Fatalf("CleanResidualItems returned unexpected err: %v", err)
	}
	if res.DeletedFilesCount != 0 || len(res.Errors) == 0 {
		t.Errorf("expected forged cleanup item to be rejected with error, got deleted=%d errors=%v", res.DeletedFilesCount, res.Errors)
	}
}

func TestProcessActionsGuardProtectedPIDs(t *testing.T) {
	self := uint32(os.Getpid())
	for _, pid := range []uint32{0, 4, self} {
		if err := SuspendProcess(pid); err == nil {
			t.Errorf("SuspendProcess(%d) should have been blocked", pid)
		}
		if err := ResumeProcess(pid); err == nil {
			t.Errorf("ResumeProcess(%d) should have been blocked", pid)
		}
		if err := GracefulCloseProcess(pid); err == nil {
			t.Errorf("GracefulCloseProcess(%d) should have been blocked", pid)
		}
		if err := KillProcess(pid); err == nil {
			t.Errorf("KillProcess(%d) should have been blocked", pid)
		}
	}
}
