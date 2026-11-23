package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type SnapshotMeta struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	ItemCount int    `json:"itemCount"`
	AppCount  int    `json:"appCount"`
}

type ComponentDiff struct {
	Component   Component `json:"component"`
	BeforeState string    `json:"beforeState"`
	AfterState  string    `json:"afterState"`
}

type SnapshotDiff struct {
	BaseID     string          `json:"baseId"`
	TargetID   string          `json:"targetId"`
	BaseName   string          `json:"baseName"`
	TargetName string          `json:"targetName"`
	Added      []Component     `json:"added"`
	Removed    []Component     `json:"removed"`
	Modified   []ComponentDiff `json:"modified"`
	Summary    string          `json:"summary"`
}

var snapMu sync.Mutex

func snapshotsDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.TempDir()
	}
	p := filepath.Join(d, "WinBGM", "snapshots")
	_ = os.MkdirAll(p, 0755)
	return p
}

// SaveSnapshot saves a baseline snapshot to disk.
func SaveSnapshot(name string, inv Inventory) (SnapshotMeta, error) {
	snapMu.Lock()
	defer snapMu.Unlock()

	if name == "" {
		name = "Snapshot " + time.Now().Format("2006-01-02 15:04")
	}

	id := fmt.Sprintf("snap-%d", time.Now().UnixNano())
	meta := SnapshotMeta{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now().Format(time.RFC3339),
		AppCount:  len(inv.Apps),
	}

	totalComps := 0
	for _, a := range inv.Apps {
		totalComps += len(a.Components)
	}
	meta.ItemCount = totalComps

	filePath := filepath.Join(snapshotsDir(), id+".json")
	data, err := json.MarshalIndent(struct {
		Meta      SnapshotMeta `json:"meta"`
		Inventory Inventory    `json:"inventory"`
	}{Meta: meta, Inventory: inv}, "", "  ")
	if err != nil {
		return meta, err
	}

	return meta, os.WriteFile(filePath, data, 0644)
}

// ListSnapshots returns all saved baseline snapshots, newest first.
func ListSnapshots() ([]SnapshotMeta, error) {
	snapMu.Lock()
	defer snapMu.Unlock()

	dir := snapshotsDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var list []SnapshotMeta
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			full := filepath.Join(dir, f.Name())
			b, err := os.ReadFile(full)
			if err != nil {
				continue
			}
			var wrapper struct {
				Meta SnapshotMeta `json:"meta"`
			}
			if json.Unmarshal(b, &wrapper) == nil && wrapper.Meta.ID != "" {
				list = append(list, wrapper.Meta)
			}
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt > list[j].CreatedAt
	})
	return list, nil
}

// LoadSnapshot reads a full snapshot by ID.
func LoadSnapshot(id string) (*Inventory, *SnapshotMeta, error) {
	snapMu.Lock()
	defer snapMu.Unlock()

	filePath := filepath.Join(snapshotsDir(), id+".json")
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, err
	}
	var wrapper struct {
		Meta      SnapshotMeta `json:"meta"`
		Inventory Inventory    `json:"inventory"`
	}
	if err := json.Unmarshal(b, &wrapper); err != nil {
		return nil, nil, err
	}
	return &wrapper.Inventory, &wrapper.Meta, nil
}

// DeleteSnapshot deletes a saved snapshot file.
func DeleteSnapshot(id string) error {
	snapMu.Lock()
	defer snapMu.Unlock()
	return os.Remove(filepath.Join(snapshotsDir(), id+".json"))
}

// DiffSnapshots compares a baseline snapshot against another snapshot or the live scan.
func DiffSnapshots(baseID string, currentInv *Inventory) (SnapshotDiff, error) {
	baseInv, baseMeta, err := LoadSnapshot(baseID)
	if err != nil {
		return SnapshotDiff{}, fmt.Errorf("could not load baseline snapshot: %w", err)
	}

	diff := SnapshotDiff{
		BaseID:     baseID,
		TargetID:   "current",
		BaseName:   baseMeta.Name,
		TargetName: "Current System State",
	}

	baseMap := map[string]Component{}
	for _, a := range baseInv.Apps {
		for _, c := range a.Components {
			baseMap[c.ID] = c
		}
	}

	currMap := map[string]Component{}
	if currentInv != nil {
		for _, a := range currentInv.Apps {
			for _, c := range a.Components {
				currMap[c.ID] = c
			}
		}
	}

	// Find added and modified components
	for id, currComp := range currMap {
		if baseComp, exists := baseMap[id]; !exists {
			diff.Added = append(diff.Added, currComp)
		} else {
			if baseComp.Enabled != currComp.Enabled || baseComp.AutoStart != currComp.AutoStart {
				var beforeParts, afterParts []string
				if baseComp.Enabled != currComp.Enabled {
					if baseComp.Enabled {
						beforeParts = append(beforeParts, "Enabled")
						afterParts = append(afterParts, "Disabled")
					} else {
						beforeParts = append(beforeParts, "Disabled")
						afterParts = append(afterParts, "Enabled")
					}
				}
				if baseComp.AutoStart != currComp.AutoStart {
					if baseComp.AutoStart {
						beforeParts = append(beforeParts, "Autostart: Yes")
						afterParts = append(afterParts, "Autostart: No")
					} else {
						beforeParts = append(beforeParts, "Autostart: No")
						afterParts = append(afterParts, "Autostart: Yes")
					}
				}
				diff.Modified = append(diff.Modified, ComponentDiff{
					Component:   currComp,
					BeforeState: strings.Join(beforeParts, ", "),
					AfterState:  strings.Join(afterParts, ", "),
				})
			}
		}
	}

	// Find removed components
	for id, baseComp := range baseMap {
		if _, exists := currMap[id]; !exists {
			diff.Removed = append(diff.Removed, baseComp)
		}
	}

	diff.Summary = fmt.Sprintf("Comparison against %q: %d new item(s), %d removed, %d modified.",
		baseMeta.Name, len(diff.Added), len(diff.Removed), len(diff.Modified))

	return diff, nil
}
