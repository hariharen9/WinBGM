package backend

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// requireLiveRecycle skips live Recycle Bin tests unless explicitly enabled:
// they touch the machine's real Recycle Bin, so they must never run as part
// of an ordinary `go test` run. Opt in with:
//
//	WINBGM_LIVE_RECYCLE_TEST=1 go test ./backend/ -run Live -v
func requireLiveRecycle(t *testing.T) {
	t.Helper()
	if os.Getenv("WINBGM_LIVE_RECYCLE_TEST") == "" {
		t.Skip("live Recycle Bin test — set WINBGM_LIVE_RECYCLE_TEST=1 to run (touches the real Recycle Bin)")
	}
}

// TestShellRecycleLive verifies the real SHFileOperationW recycle path on this
// machine: a scratch file must disappear from disk and appear in the Recycle
// Bin (SHQueryRecycleBinW count increases). Skipped if the shell refuses.
func TestShellRecycleLive(t *testing.T) {
	requireLiveRecycle(t)
	dir, err := os.MkdirTemp("", "winbgm-recycle-diag")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(dir)

	f := filepath.Join(dir, "diag-old.txt")
	if err := os.WriteFile(f, []byte("recycle diagnostic"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	beforeSize, beforeCount := queryRecycleBinSize()
	t.Logf("recycle bin before: %d items, %d bytes", beforeCount, beforeSize)

	if err := shellRecycleBatch([]string{f}); err != nil {
		t.Fatalf("shellRecycleBatch FAILED: %v", err)
	}

	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Errorf("file still exists on disk after recycle")
	}

	afterSize, afterCount := queryRecycleBinSize()
	t.Logf("recycle bin after: %d items, %d bytes", afterCount, afterSize)
	if afterCount <= beforeCount {
		t.Errorf("recycle bin item count did not increase (%d -> %d) — file did NOT land in the bin", beforeCount, afterCount)
	}
}

// TestRecycleJunkPathsLive exercises the batched path used by the cleaner.
func TestRecycleJunkPathsLive(t *testing.T) {
	requireLiveRecycle(t)
	dir, err := os.MkdirTemp("", "winbgm-recycle-batch-diag")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(dir)

	var targets []string
	for i := 0; i < 70; i++ { // spans two batches (64 + 6)
		f := filepath.Join(dir, "f"+time.Now().Format("150405")+"-"+string(rune('a'+i%26))+string(rune('a'+i/26))+".tmp")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		targets = append(targets, f)
	}

	freed, files, errs := recycleJunkPaths(targets)
	t.Logf("freed=%d files=%d errs=%v", freed, files, errs)
	if files != len(targets) {
		t.Errorf("expected %d files recycled, got %d (errs: %v)", len(targets), files, errs)
	}
}
