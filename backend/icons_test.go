package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseIconSpec(t *testing.T) {
	p, idx := parseIconSpec(`"C:\Program Files\App\app.exe",2`)
	if p != `C:\Program Files\App\app.exe` || idx != 2 {
		t.Fatalf("got (%q, %d), want (%q, 2)", p, idx, `C:\Program Files\App\app.exe`)
	}
	p2, idx2 := parseIconSpec(`C:\Windows\explorer.exe`)
	if p2 != `C:\Windows\explorer.exe` || idx2 != 0 {
		t.Fatalf("got (%q, %d), want (%q, 0)", p2, idx2, `C:\Windows\explorer.exe`)
	}
}

func TestGetAppIconsExplorer(t *testing.T) {
	explorer := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	icons := GetAppIcons([]string{explorer, `C:\nonexistent\missing.exe`})
	uri := icons[strings.ToLower(explorer)]
	if !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Fatalf("expected PNG data URI for explorer.exe, got %q", uri)
	}
	if _, ok := icons[`c:\nonexistent\missing.exe`]; ok {
		t.Error("expected missing.exe not to produce an icon entry")
	}
}
