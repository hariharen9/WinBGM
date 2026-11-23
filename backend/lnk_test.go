package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveShortcut(t *testing.T) {
	startupDir := filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Startup`)
	files, err := os.ReadDir(startupDir)
	if err != nil {
		t.Skip("Startup directory not accessible")
	}
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".lnk" {
			full := filepath.Join(startupDir, f.Name())
			target, args := resolveShortcut(full)
			t.Logf("LNK %s -> Target: %q, Args: %q", f.Name(), target, args)
			if target == "" {
				t.Errorf("Failed to resolve target for %s", f.Name())
			}
		}
	}
}
