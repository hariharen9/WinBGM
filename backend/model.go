package backend

import "fmt"

type Component struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"` // service | run | folder | task | process | storeapp | winlogon | wmi
	Name          string   `json:"name"`
	Display       string   `json:"display"`
	Command       string   `json:"command"`
	ExePath       string   `json:"exePath"`
	Source        string   `json:"source"`
	Trigger       string   `json:"trigger"`
	StartType     string   `json:"startType"`
	Enabled       bool     `json:"enabled"`   // configured on (not disabled)
	AutoStart     bool     `json:"autoStart"` // launches by itself at boot/sign-in
	Running       bool     `json:"running"`
	PIDs          []uint32 `json:"pids"`
	RAMBytes      uint64   `json:"ramBytes"`
	RAMFormatted  string   `json:"ramFormatted"`
	SHA256        string   `json:"sha256"`
	VirusTotalURL string   `json:"virusTotalURL"`
	LastRun       string   `json:"lastRun"`
	Publisher     string   `json:"publisher"`
	Product       string   `json:"product"`
	Signature     string   `json:"signature"` // valid | unsigned | missing | n/a
	Signer        string   `json:"signer"`    // e.g. "Google LLC", "Valve Corp."
	Locked        string   `json:"locked"`    // "" | "yes" | "unknown"
	System        bool     `json:"system"`
	Review        bool     `json:"review"`
	Flags         []string `json:"flags"`
	Control       string   `json:"control"`     // yes | admin | no  (can its startup setting be changed here?)
	ControlNote   string   `json:"controlNote"` // why not
	Caution       string   `json:"caution"`
	CanStop       bool     `json:"canStop"`

	tgt                  Target
	ownerKey, ownerLabel string
	ownerSystem          bool
}

type AppGroup struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Publisher    string      `json:"publisher"`
	Signer       string      `json:"signer"`
	Letter       string      `json:"letter"`
	Signature    string      `json:"signature"` // verified | unverified | unknown
	Path         string      `json:"path"`
	Components   []Component `json:"components"`
	Running      bool        `json:"running"`
	RAMBytes     uint64      `json:"ramBytes"`
	RAMFormatted string      `json:"ramFormatted"`
	AutoStart    bool        `json:"autoStart"`
	Review       bool        `json:"review"`
	System       bool        `json:"system"`
	Behavior     string      `json:"behavior"`
	BehaviorNote string      `json:"behaviorNote"`
}

type SourceInfo struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Desc   string `json:"desc"`
	Count  int    `json:"count"`
	Status string `json:"status"` // scanned | partial | notscanned
}

type Inventory struct {
	Apps              []AppGroup   `json:"apps"`
	Sources           []SourceInfo `json:"sources"`
	Warnings          []string     `json:"warnings"`
	ScannedAt         string       `json:"scannedAt"`
	DurationMs        int64        `json:"durationMs"`
	Machine           string       `json:"machine"`
	OS                string       `json:"os"`
	Elevated          bool         `json:"elevated"`
	AlwaysAdmin       bool         `json:"alwaysAdmin"`
	TotalRAMBytes     uint64       `json:"totalRAMBytes"`
	TotalRAMFormatted string       `json:"totalRAMFormatted"`
}

func formatRAM(bytes uint64) string {
	if bytes == 0 {
		return ""
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	default:
		return fmt.Sprintf("%d KB", bytes/kb)
	}
}
