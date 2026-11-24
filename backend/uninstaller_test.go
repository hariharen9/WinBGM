package backend

import (
	"testing"
)

func TestScanInstalledApps(t *testing.T) {
	apps, summary, err := ScanInstalledApps()
	if err != nil {
		t.Fatalf("ScanInstalledApps failed: %v", err)
	}

	if len(apps) == 0 {
		t.Logf("No apps found (might be headless test environment)")
	} else {
		t.Logf("Found %d installed apps, total size: %s", summary.TotalApps, summary.TotalSizeFormatted)
		for i, a := range apps {
			if i < 5 {
				t.Logf("  [%d] %s (v%s) by %s - Size: %s - Heavy: %v", i+1, a.Name, a.Version, a.Publisher, a.DisplaySize, a.HasHeavyProfile)
			}
		}
	}
}

func TestHeavyProfilesMatching(t *testing.T) {
	// Test Ollama matching
	matches := FindMatchingProfiles("Ollama", "ollama")
	if len(matches) == 0 {
		t.Errorf("Expected to match Ollama profile, got none")
	} else if matches[0].Key != "ollama" {
		t.Errorf("Expected profile key 'ollama', got %s", matches[0].Key)
	}

	// Test Android Studio matching
	matchesAndroid := FindMatchingProfiles("Android Studio", "AndroidStudio")
	if len(matchesAndroid) == 0 {
		t.Errorf("Expected to match Android Studio profile, got none")
	}

	// Test VS Code matching
	matchesCode := FindMatchingProfiles("Microsoft Visual Studio Code", "vscode")
	if len(matchesCode) == 0 {
		t.Errorf("Expected to match VS Code profile, got none")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected string
	}{
		{0, "—"},
		{500, "500 B"},
		{1024 * 50, "50.0 KB"},
		{1024 * 1024 * 350, "350.0 MB"},
		{1024 * 1024 * 1024 * 8, "8.00 GB"},
	}

	for _, tc := range tests {
		actual := FormatBytes(tc.bytes)
		if actual != tc.expected {
			t.Errorf("FormatBytes(%d) = %s; want %s", tc.bytes, actual, tc.expected)
		}
	}
}

func TestScanResidualsOllama(t *testing.T) {
	app := InstalledApp{
		ID:   "HKCU_Ollama",
		Name: "Ollama",
	}

	report, err := ScanResiduals(app)
	if err != nil {
		t.Fatalf("ScanResiduals failed: %v", err)
	}

	t.Logf("Ollama Residual Report: total items=%d, total size=%s, heavy size=%s",
		len(report.Items), report.TotalSizeFormatted, report.HeavySizeFormatted)

	for _, it := range report.Items {
		t.Logf("  -> [%s] %s (%s) at %s", it.Category, it.Title, it.SizeFormatted, it.Path)
	}
}

