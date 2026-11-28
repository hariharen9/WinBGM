package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	defaultWindowWidth  = 1280
	defaultWindowHeight = 820
)

// windowState persists the main window geometry across launches.
type windowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximized bool `json:"maximized"`
}

func windowStatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "WinBGM", "ui.json")
}

// loadWindowState returns the saved geometry, falling back to defaults.
// X/Y of 0 keeps Wails' default centering; a saved position is only used
// if it is plausible on screen (never buried deep off-monitor).
func loadWindowState() windowState {
	st := windowState{Width: defaultWindowWidth, Height: defaultWindowHeight}
	p := windowStatePath()
	if p == "" {
		return st
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return st
	}
	var saved windowState
	if err := json.Unmarshal(b, &saved); err != nil {
		return st
	}
	if saved.Width >= 900 && saved.Height >= 600 {
		st.Width, st.Height = saved.Width, saved.Height
	}
	// Accept saved positions only if not absurdly off-screen (e.g. a monitor
	// that has been unplugged). Windows clamps the rest.
	if saved.X > -32000 && saved.Y > -32000 && (saved.X != 0 || saved.Y != 0) {
		st.X, st.Y = saved.X, saved.Y
	}
	st.Maximized = saved.Maximized
	return st
}

// applyWindowState restores the saved geometry once the window exists.
func (a *App) applyWindowState(st windowState) {
	if st.X != 0 || st.Y != 0 {
		runtime.WindowSetPosition(a.ctx, st.X, st.Y)
	}
	runtime.WindowSetSize(a.ctx, st.Width, st.Height)
	if st.Maximized {
		runtime.WindowMaximise(a.ctx)
	}
}

// saveWindowState records the current geometry so the next launch feels identical.
func (a *App) saveWindowState() {
	p := windowStatePath()
	if p == "" {
		return
	}
	if runtime.WindowIsMinimised(a.ctx) {
		return // Minimized geometry is meaningless; keep the last good state.
	}
	w, h := runtime.WindowGetSize(a.ctx)
	x, y := runtime.WindowGetPosition(a.ctx)
	st := windowState{Width: w, Height: h, X: x, Y: y, Maximized: runtime.WindowIsMaximised(a.ctx)}
	if st.Width < 900 || st.Height < 600 {
		return
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, b, 0o644)
}
