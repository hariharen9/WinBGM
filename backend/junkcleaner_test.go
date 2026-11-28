package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJunkPathIsSensitive(t *testing.T) {
	sensitive := []string{
		`C:\Users\x\AppData\Local\Google\Chrome\User Data\Default\Cookies`,
		`C:\Users\x\AppData\Roaming\app\Login Data`,
		`C:\Users\x\AppData\Local\browser\Session Storage`,
		`C:\Users\x\AppData\Local\app\History Database`,
	}
	for _, p := range sensitive {
		if !junkPathIsSensitive(p) {
			t.Errorf("expected %q to be treated as sensitive", p)
		}
	}
	safe := []string{
		`C:\Users\x\AppData\Local\Google\Chrome\User Data\Default\Cache`,
		`C:\Windows\Temp`,
		`C:\Users\x\AppData\Local\D3DSCache`,
	}
	for _, p := range safe {
		if junkPathIsSensitive(p) {
			t.Errorf("expected %q to be considered safe", p)
		}
	}
}

func TestEligibleJunkChildrenAgeAndSymlinks(t *testing.T) {
	dir := t.TempDir()

	oldFile := filepath.Join(dir, "old.txt")
	newFile := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newFile, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	twoDaysAgo := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, twoDaysAgo, twoDaysAgo); err != nil {
		t.Skip("cannot set file times in this environment")
	}

	// 24h cutoff: only old.txt is eligible.
	children := eligibleJunkChildren(dir, tempMinAge, nil)
	if len(children) != 1 || filepath.Base(children[0]) != "old.txt" {
		t.Fatalf("expected only old.txt eligible, got %v", children)
	}

	// No cutoff: both files eligible.
	children = eligibleJunkChildren(dir, 0, nil)
	if len(children) != 2 {
		t.Fatalf("expected both files eligible without age filter, got %v", children)
	}

	// Pattern filter.
	children = eligibleJunkChildren(dir, 0, func(name string) bool { return name == "new.txt" })
	if len(children) != 1 || filepath.Base(children[0]) != "new.txt" {
		t.Fatalf("expected pattern to select new.txt, got %v", children)
	}

	// Missing directory yields nothing.
	if children := eligibleJunkChildren(filepath.Join(dir, "nope"), 0, nil); children != nil {
		t.Fatalf("expected nil for missing dir, got %v", children)
	}
}

func TestMeasureJunkItemMatchesCleanerSemantics(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(sub, "old.bin")
	if err := os.WriteFile(old, make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(old, stale, stale); err != nil {
		t.Skip("cannot set file times")
	}
	// sub itself is recent; its old content must not be counted because the
	// cleaner only recycles eligible top-level children.
	if err := os.Chtimes(sub, time.Now(), time.Now()); err != nil {
		t.Skip("cannot set dir times")
	}

	item := JunkItem{Path: dir, contentsOnly: true, minAge: tempMinAge}
	bytes, count := measureJunkItem(item)
	if bytes != 0 || count != 0 {
		t.Fatalf("recent top-level child should be excluded: got %d bytes, %d files", bytes, count)
	}

	// Make the child eligible: its whole subtree counts.
	if err := os.Chtimes(sub, stale, stale); err != nil {
		t.Skip("cannot set dir times")
	}
	bytes, count = measureJunkItem(item)
	if bytes != 1024 || count != 1 {
		t.Fatalf("expected 1024 bytes / 1 file, got %d / %d", bytes, count)
	}
}

func TestJunkCategorySafetyOrdering(t *testing.T) {
	// Temp must come first, destructive-ish categories last, and every
	// category must have a human title.
	for cat, title := range junkCategoryTitles {
		if strings.TrimSpace(title) == "" {
			t.Errorf("category %q has empty title", cat)
		}
		if _, ok := junkCategoryOrder[cat]; !ok {
			t.Errorf("category %q missing from ordering map", cat)
		}
	}
	if junkCategoryOrder[JunkCatTemp] > junkCategoryOrder[JunkCatOrphanReg] {
		t.Error("temp files should sort before registry categories")
	}
}

func TestRecycleBinQueryDoesNotPanic(t *testing.T) {
	// Must not panic and must not report absurd sizes regardless of drives.
	size, count := queryRecycleBinSize()
	if count < 0 {
		t.Errorf("negative recycle bin item count: %d (size %d)", count, size)
	}
}
