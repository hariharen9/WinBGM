# Background Manager (read-only)

Go + Wails desktop app. Reads services, Run keys, Startup folders, scheduled tasks, Store-app
background permissions and windowless running processes, groups them by owning app, checks
signatures, and shows it in the UI from your mockup. It never changes anything.

## Prerequisites (Windows 10/11)
- Go 1.22+
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- WebView2 runtime (already present on Windows 11)
- Check with: `wails doctor`

## Run
```
cd bgm-ui
go mod tidy
wails dev          # live window
wails build        # -> build\bin\BackgroundManager.exe
```
Right-click the exe -> Run as administrator for the complete scan (all processes visible,
and "Locked" status for services).

## UI-only preview
Open `frontend/dist/index.html` in a browser: it shows sample data.

## Layout
- inventory.go  scan orchestration, grouping, review rules
- services.go / registry.go / tasks.go  data sources
- procs.go  process table, windowless-process heuristic
- pe.go  publisher name (version info) + Authenticode check
- owners.go  maps an exe path to an app
- frontend/dist/index.html  the whole UI (no build step)
