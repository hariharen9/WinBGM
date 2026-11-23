package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShellExtensionsScanner(t *testing.T) {
	pt := snapshotProcs()
	comps, err := scanShellExtensions(pt, false)
	if err != nil {
		t.Fatalf("scanShellExtensions failed: %v", err)
	}
	t.Logf("Found %d Explorer shell extension components", len(comps))
	for _, c := range comps {
		if c.Kind != "shellex" {
			t.Errorf("expected kind 'shellex', got %q", c.Kind)
		}
		if c.ID == "" {
			t.Errorf("empty ID for component %q", c.Name)
		}
	}
}

func TestNativeMessagingScanner(t *testing.T) {
	pt := snapshotProcs()
	comps, err := scanNativeMessaging(pt, false)
	if err != nil {
		t.Fatalf("scanNativeMessaging failed: %v", err)
	}
	t.Logf("Found %d Native Messaging components", len(comps))
	for _, c := range comps {
		if c.Kind != "nativemessaging" {
			t.Errorf("expected kind 'nativemessaging', got %q", c.Kind)
		}
	}
}

func TestDriversScanner(t *testing.T) {
	pt := snapshotProcs()
	comps, err := scanDrivers(pt, false)
	if err != nil {
		t.Fatalf("scanDrivers failed: %v", err)
	}
	t.Logf("Found %d 3rd party kernel/hardware driver components", len(comps))
	for _, c := range comps {
		if c.Kind != "driver" {
			t.Errorf("expected kind 'driver', got %q", c.Kind)
		}
	}
}

func TestActiveSetupScanner(t *testing.T) {
	pt := snapshotProcs()
	comps, err := scanActiveSetup(pt, false)
	if err != nil {
		t.Fatalf("scanActiveSetup failed: %v", err)
	}
	t.Logf("Found %d Active Setup components", len(comps))
	for _, c := range comps {
		if c.Kind != "activesetup" {
			t.Errorf("expected kind 'activesetup', got %q", c.Kind)
		}
	}
}

func TestSnapshotLifecycleAndDiff(t *testing.T) {
	// 1. Create a dummy test inventory
	inv1 := Inventory{
		Machine: "TEST-PC",
		OS:      "Windows 11",
		Apps: []AppGroup{
			{
				ID:   "app1",
				Name: "TestApp",
				Components: []Component{
					{
						ID:      "comp1",
						Name:    "TestService",
						Kind:    "service",
						Enabled: true,
					},
					{
						ID:      "comp2",
						Name:    "TestRun",
						Kind:    "run",
						Enabled: true,
					},
				},
			},
		},
	}

	// 2. Save snapshot
	meta, err := SaveSnapshot("Test Baseline", inv1)
	if err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}
	defer DeleteSnapshot(meta.ID)

	if meta.ItemCount != 2 {
		t.Errorf("expected 2 items in meta, got %d", meta.ItemCount)
	}

	// 3. List snapshots
	list, err := ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	found := false
	for _, s := range list {
		if s.ID == meta.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("snapshot %s not found in list", meta.ID)
	}

	// 4. Create modified inventory for diff
	inv2 := Inventory{
		Machine: "TEST-PC",
		OS:      "Windows 11",
		Apps: []AppGroup{
			{
				ID:   "app1",
				Name: "TestApp",
				Components: []Component{
					{
						ID:      "comp1",
						Name:    "TestService",
						Kind:    "service",
						Enabled: false, // Modified state!
					},
					{
						ID:      "comp3",
						Name:    "NewMalwareOrApp", // Added item!
						Kind:    "run",
						Enabled: true,
					},
					// comp2 removed!
				},
			},
		},
	}

	// 5. Run Diff
	diff, err := DiffSnapshots(meta.ID, &inv2)
	if err != nil {
		t.Fatalf("DiffSnapshots failed: %v", err)
	}

	if len(diff.Added) != 1 || diff.Added[0].ID != "comp3" {
		t.Errorf("expected 1 added component (comp3), got %+v", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].ID != "comp2" {
		t.Errorf("expected 1 removed component (comp2), got %+v", diff.Removed)
	}
	if len(diff.Modified) != 1 || diff.Modified[0].Component.ID != "comp1" {
		t.Errorf("expected 1 modified component (comp1), got %+v", diff.Modified)
	}
}

func TestAppxManifestParser(t *testing.T) {
	// Create a temporary mock package directory with AppxManifest.xml
	tmpDir, err := os.MkdirTemp("", "WindowsApps_mock")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pkgPath := filepath.Join(tmpDir, "WindowsApps", "TestPublisher.MockApp_1.0.0.0_x64__123456")
	_ = os.MkdirAll(pkgPath, 0755)

	manifestContent := `<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10">
  <Identity Name="TestPublisher.MockApp" Publisher="CN=Test" Version="1.2.3.4" />
  <Properties>
    <DisplayName>Mock Application</DisplayName>
    <PublisherDisplayName>Test Publisher Inc</PublisherDisplayName>
  </Properties>
</Package>`

	_ = os.WriteFile(filepath.Join(pkgPath, "AppxManifest.xml"), []byte(manifestContent), 0644)
	mockExe := filepath.Join(pkgPath, "MockApp.exe")

	info, ok := ResolveAppxFromPath(mockExe)
	if !ok {
		t.Fatalf("expected ResolveAppxFromPath to succeed")
	}

	if info.DisplayName != "Mock Application" {
		t.Errorf("expected 'Mock Application', got %q", info.DisplayName)
	}
	if info.Publisher != "Test Publisher Inc" {
		t.Errorf("expected 'Test Publisher Inc', got %q", info.Publisher)
	}
	if info.Version != "1.2.3.4" {
		t.Errorf("expected '1.2.3.4', got %q", info.Version)
	}
}

func TestSentinelAlerts(t *testing.T) {
	ClearSentinelAlerts()
	alerts := GetSentinelAlerts()
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts initially, got %d", len(alerts))
	}
}

func TestProcessActionsSuspension(t *testing.T) {
	if IsProcessSuspended(9999999) {
		t.Errorf("expected PID 9999999 not to be marked suspended")
	}
}
