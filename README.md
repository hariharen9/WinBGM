# Background Manager

Go + Wails desktop app: see what runs in the background on Windows, who owns it, and change it safely.

## Prerequisites (Windows 10/11)
- Go 1.22+
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- WebView2 runtime (already on Windows 11); check with `wails doctor`

## Run
```
cd bgm-ui
go mod tidy
wails dev          # live window
wails build        # -> build\bin\BackgroundManager.exe
```
Open `frontend/dist/index.html` in a browser for a UI-only preview with sample data.

## What it can change (all reversible, nothing is ever deleted)
| Item | Disable does | Enable does |
|---|---|---|
| Registry Run / Startup folder | Task Manager's StartupApproved flag -> off | flag -> on |
| Scheduled task | Disable-ScheduledTask | Enable-ScheduledTask |
| Service | startup type -> Manual (not Disabled) | -> Automatic |
| Service (extra) | Stop now | Start now |
Processes and Store-app permissions are shown with "How to change" notes only.

## Safety rules (enforced in Go, the UI is never trusted)
1. Windows components, critical services, antivirus/security software and the app itself are blocked.
2. Services that deny reconfiguration are marked Locked and not touched (stop/start is still offered).
3. The item is re-read live; if it differs from the last scan the change is refused.
4. The original value is written to `%APPDATA%\BackgroundManager\history.json` BEFORE the change. If that fails, nothing changes.
5. After the change it is read back; if it did not take effect the original is put back automatically.
6. Stopping a service is refused while running services depend on it.
7. Restore only runs if the item is still exactly as we left it, and restores the exact original bytes.
8. Services, tasks and HKLM items need administrator rights ("Restart as administrator" button).

## Suggested first test
Pick something harmless you recognize (e.g. an app in HKCU Run), disable it, check Task Manager > Startup apps
shows it Disabled, then Restore from Change history and confirm it is Enabled again.

## Layout
inventory.go (scan+grouping) · services.go/registry.go/tasks.go (sources) · procs.go · pe.go (publisher+signature) ·
owners.go · protect.go (safety rules) · ops.go (apply/restore) · history.go (backups) · frontend/dist/index.html (UI)
