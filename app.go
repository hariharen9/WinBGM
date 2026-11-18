package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is bound to the frontend. Every method here is read-only with respect to Windows settings.
type App struct {
	ctx  context.Context
	last *Inventory
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// Scan inspects the machine and returns the full inventory. It changes nothing.
func (a *App) Scan() Inventory {
	inv := buildInventory()
	a.last = &inv
	return inv
}

// Reveal opens File Explorer with the given file selected (or its folder if the file is gone).
func (a *App) Reveal(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("no location available")
	}
	target := path
	if _, err := os.Stat(path); err != nil {
		target = filepath.Dir(path)
		if _, err := os.Stat(target); err != nil {
			return errors.New("location not found")
		}
		c := exec.Command("explorer.exe", target)
		return c.Start()
	}
	c := exec.Command("explorer.exe")
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + target + `"`}
	return c.Start()
}

// Export writes the last scan to a JSON file chosen by the user. Returns the path ("" if cancelled).
func (a *App) Export() (string, error) {
	if a.last == nil {
		return "", errors.New("run a scan first")
	}
	p, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export background inventory",
		DefaultFilename: "background-inventory-" + time.Now().Format("2006-01-02") + ".json",
		Filters:         []runtime.FileFilter{{DisplayName: "JSON (*.json)", Pattern: "*.json"}},
	})
	if err != nil || p == "" {
		return "", err
	}
	b, err := json.MarshalIndent(a.last, "", "  ")
	if err != nil {
		return "", err
	}
	return p, os.WriteFile(p, b, 0o644)
}
