package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type HistoryEntry struct {
	ID            string `json:"id"`
	Time          string `json:"time"`
	Title         string `json:"title"`
	AppName       string `json:"appName"`
	Kind          string `json:"kind"`
	Action        string `json:"action"`
	Summary       string `json:"summary"`
	Target        Target `json:"target"`
	Before        string `json:"before"`
	After         string `json:"after"`
	BeforeRaw     string `json:"beforeRaw"`
	BeforeExisted bool   `json:"beforeExisted"`
	Status        string `json:"status"` // pending | applied | failed | rolledback | restored
	Restorable    bool   `json:"restorable"`
	RestoredAt    string `json:"restoredAt"`
	Error         string `json:"error"`
}

var histMu sync.Mutex

func historyPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.TempDir()
	}
	newPath := filepath.Join(d, "WinBGM", "history.json")
	oldPath := filepath.Join(d, "BackgroundManager", "history.json")
	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}
	if _, err := os.Stat(oldPath); err == nil {
		_ = os.MkdirAll(filepath.Dir(newPath), 0755)
		_ = os.Rename(oldPath, newPath)
		return newPath
	}
	return newPath
}

func loadHistory() []HistoryEntry {
	histMu.Lock()
	defer histMu.Unlock()
	return loadHistoryLocked()
}

func loadHistoryLocked() []HistoryEntry {
	b, err := os.ReadFile(historyPath())
	if err != nil {
		return nil
	}
	var h []HistoryEntry
	if json.Unmarshal(b, &h) != nil {
		return nil
	}
	return h
}

// saveHistoryLocked writes atomically (temp file + rename) so a crash can't corrupt the backup.
func saveHistoryLocked(h []HistoryEntry) error {
	p := historyPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func appendHistory(e HistoryEntry) error {
	histMu.Lock()
	defer histMu.Unlock()
	return saveHistoryLocked(append(loadHistoryLocked(), e))
}

func updateHistory(e HistoryEntry) error {
	histMu.Lock()
	defer histMu.Unlock()
	h := loadHistoryLocked()
	for i := range h {
		if h[i].ID == e.ID {
			h[i] = e
			return saveHistoryLocked(h)
		}
	}
	return saveHistoryLocked(append(h, e))
}
