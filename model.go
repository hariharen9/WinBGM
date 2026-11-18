package main

type Component struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"` // service | run | folder | task | process | storeapp
	Name      string   `json:"name"`
	Display   string   `json:"display"`
	Command   string   `json:"command"`
	ExePath   string   `json:"exePath"`
	Source    string   `json:"source"`
	Trigger   string   `json:"trigger"`
	StartType string   `json:"startType"`
	Enabled   bool     `json:"enabled"`   // configured on (not disabled)
	AutoStart bool     `json:"autoStart"` // launches by itself at boot/sign-in
	Running   bool     `json:"running"`
	PIDs      []uint32 `json:"pids"`
	LastRun   string   `json:"lastRun"`
	Publisher string   `json:"publisher"`
	Product   string   `json:"product"`
	Signature string   `json:"signature"` // valid | unsigned | missing | n/a
	Locked    string   `json:"locked"`    // "" | "yes" | "unknown"
	System    bool     `json:"system"`
	Review    bool     `json:"review"`
	Flags     []string `json:"flags"`

	ownerKey, ownerLabel string
	ownerSystem          bool
}

type AppGroup struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Publisher    string      `json:"publisher"`
	Letter       string      `json:"letter"`
	Signature    string      `json:"signature"` // verified | unverified | unknown
	Path         string      `json:"path"`
	Components   []Component `json:"components"`
	Running      bool        `json:"running"`
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
	Apps       []AppGroup   `json:"apps"`
	Sources    []SourceInfo `json:"sources"`
	Warnings   []string     `json:"warnings"`
	ScannedAt  string       `json:"scannedAt"`
	DurationMs int64        `json:"durationMs"`
	Machine    string       `json:"machine"`
	OS         string       `json:"os"`
	Elevated   bool         `json:"elevated"`
}
