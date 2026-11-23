package backend

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestHistoryBatchAndRollback(t *testing.T) {
	// Use isolated temp history file for test
	tempDir, err := os.MkdirTemp("", "winbgm-hist-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	batchID := fmt.Sprintf("test-batch-%d", time.Now().UnixNano())

	// Add test entries with this batch ID
	entry1 := HistoryEntry{
		ID:         fmt.Sprintf("test-%d-1", time.Now().UnixNano()),
		BatchID:    batchID,
		Time:       time.Now().Format(time.RFC3339),
		Title:      "Test App 1",
		AppName:    "Test Suite",
		Kind:       "run",
		Action:     "disable",
		Summary:    "Disable Test App 1",
		Status:     "applied",
		Restorable: true,
	}

	entry2 := HistoryEntry{
		ID:         fmt.Sprintf("test-%d-2", time.Now().UnixNano()),
		BatchID:    batchID,
		Time:       time.Now().Format(time.RFC3339),
		Title:      "Test App 2",
		AppName:    "Test Suite",
		Kind:       "run",
		Action:     "disable",
		Summary:    "Disable Test App 2",
		Status:     "applied",
		Restorable: true,
	}

	if err := appendHistory(entry1); err != nil {
		t.Fatalf("Failed to append history: %v", err)
	}
	if err := appendHistory(entry2); err != nil {
		t.Fatalf("Failed to append history: %v", err)
	}

	// Verify loaded history contains both entries with correct BatchID
	hist := loadHistory()
	found1, found2 := false, false
	for _, h := range hist {
		if h.ID == entry1.ID && h.BatchID == batchID {
			found1 = true
		}
		if h.ID == entry2.ID && h.BatchID == batchID {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Errorf("Expected both test entries to be present in history with batchID %s", batchID)
	}

	// Clean up added test entries
	histMu.Lock()
	var cleaned []HistoryEntry
	for _, h := range loadHistoryLocked() {
		if h.ID != entry1.ID && h.ID != entry2.ID {
			cleaned = append(cleaned, h)
		}
	}
	_ = saveHistoryLocked(cleaned)
	histMu.Unlock()
}
