<div align="center">

# <img src="assets/WinBGM.png" alt="WinBGM Logo" width="38" height="38" align="absmiddle" style="vertical-align: middle;" /> WinBGM

**Windows Background Manager**  
_A fast, safe, all-in-one startup, background process, deep uninstaller, junk cleaner, winget updates, and system diagnostics manager for Windows 10 and 11._

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Wails v2](https://img.shields.io/badge/Built%20with-Wails%20v2-E3646E?style=flat&logo=wails)](https://wails.io/)
[![Platform](https://img.shields.io/badge/Windows-10%20%7C%2011-0078D6?style=flat&logo=windows)](https://microsoft.com/windows)
[![Arch](https://img.shields.io/badge/Arch-x64%20%7C%20ARM64%20%7C%20x86-blueviolet?style=flat)]()
[![Safety](https://img.shields.io/badge/Safety-Backed%20Up%20%26%20Guarded-2ea44f?style=flat&logo=shield)]()
[![License](https://img.shields.io/badge/License-MIT-orange?style=flat)](LICENSE)

<br/>

[Capabilities](#-key-capabilities) •
[Safety & Guardrails](#-safety--guardrails) •
[Comparison](#-how-winbgm-compares) •
[Shortcuts](#-keyboard-shortcuts) •
[Quick Start](#-quick-start)

<br/>

<img src="assets/hero-image.png" alt="WinBGM — Windows Background Manager Preview" width="880" />

</div>

---

## ✨ Key Capabilities

### 🚀 Startup, Services & Custom Launch Control

- **App-Grouped Persistence**: Automatically groups startup hooks, Windows Services, Scheduled Tasks, Shell Extensions, Browser Native Messaging hosts, Active Setup, Winlogon hooks, WMI subscriptions, and third-party drivers by application—with authentic executable icons.
- **Custom & Delayed Startup Entries**: Launch any app, script, or command at sign-in with an optional boot delay (`0s` instant `HKCU\...\Run` entry or native Windows Task Scheduler logon task with `PT{N}S` delay, hidden window, and elevation options).
- **Non-Destructive Toggling**: Uses Windows' native `StartupApproved` bitmasks, demotes services to `Manual` instead of `Disabled`, and disables tasks cleanly without deleting definitions.
- **Security & Verification**: 1-click **VirusTotal SHA-256** lookup and impersonation checks for binaries masquerading as system files.

### 🧹 Deep Uninstaller & Residual Cleanup

- **Universal Software Discovery**: Audits **Win32 Registry** (64-bit, 32-bit, per-user), **Microsoft Store (Appx)**, **Steam**, **Scoop**, **Chocolatey**, and **Orphaned / Portable** folders in one unified inventory.
- **Batch Multi-Select Uninstall**: Queue multiple apps to uninstall sequentially, then review and clean all leftover files and registry keys in one unified pass.
- **Heavy Dependency Profiling**: Detects external payloads that standard uninstallers leave behind (Ollama/LM Studio/ComfyUI models, Android SDKs/AVDs, IDE extensions, media caches).
- **Safe Cleanup**: Nothing is ever pre-selected; leftover files move strictly to the **Windows Recycle Bin** (`SHFileOperationW`), and registry keys are exported to `.reg` backups before removal.

### 🧽 Junk & Cache Cleaner

- **Nine Cleanup Categories in One Scan**: User & system temp folders, browser caches (Chrome/Edge/Firefox/Brave/Opera profiles), Windows Update delivery & download cache, DirectX/GLSL/Vulkan shader caches, thumbnail & icon caches, crash dumps & old logs, `Windows.old` upgrade leftovers, the Recycle Bin itself, and orphaned uninstall registry entries.
- **Recycle-Bin-Only Deletion**: Every file and folder is moved with `SHFileOperationW` + `FOF_ALLOWUNDO` — nothing is ever permanently erased, so any cleanup is fully reversible from the Recycle Bin.
- **Sensitive Data Denylist**: Cookies, logins, saved passwords, history, sessions, bookmarks, IndexedDB and Local Storage are never offered for cleaning, even inside cache folders.
- **Freshness & Integrity Filters**: Items younger than 24 h (temp) or 7 d (logs/dumps) are skipped, symlinks/junctions are never followed, and admin-gated locations are clearly badged instead of failing silently.
- **Registry Orphan Sweep**: Detects uninstall entries whose target executables no longer exist (MSI products excluded), exports each key to a `.reg` backup, then removes it.

### ⬆️ Winget Updates

- **One-Click Package Upgrades**: Lists every application the **Windows Package Manager** can upgrade (including `msstore` apps and unknown-version entries) and updates them with `winget upgrade --id … --exact --silent`.
- **Batch Queue with Live Status**: Select multiple packages and update them one-by-one with per-row **Updating… / Updated / Failed** badges; the list auto-rescans when the queue finishes so versions reflect reality.
- **Guardrailed Updates**: Only package IDs from the most recent scan can be updated, winget runs with all agreements pre-accepted and interactivity disabled (it can never hang waiting for input in a hidden window), and WinBGM refuses to update **itself** from inside the running app.
- **Friendly Failure Reporting**: Common winget errors (hash mismatch, elevation required, cancelled UAC, pinned packages) are translated into plain English with the CLI's output tail attached.

### ⚡ Live Processes, Efficiency Mode & Network Monitor

- **Grouped Process Tree**: Monitor running apps grouped by executable with live **CPU %**, **Memory**, **Disk I/O**, and **Active Sockets**.
- **Power & Process Control**: Apply **Windows 11 Efficiency Mode (EcoQoS)** via `SetProcessInformation`, **Freeze / Resume** threads (`NtSuspendProcess`), change priority class, send graceful `WM_CLOSE`, or end tasks.
- **Network & Telemetry (`iphlpapi.dll`)**: Maps live TCP/UDP sockets to owning processes, resolves public remote endpoints, and tags telemetry/analytics connections.
- **Live Overview Telemetry**: Rolling 3-second SVG sparklines for CPU %, physical memory (`GlobalMemoryStatusEx`), and active network sockets.

### 🔍 System Diagnostics, Locks & Snapshots

- **Boot Event Profiler & Sleep Blockers**: Inspect Windows Boot Diagnostics (Event IDs 100–109) and live `powercfg /requests` wake/sleep blockers.
- **File Lock Finder (`rstrtmgr.dll`)**: Pick or paste any locked file or folder to see the exact holding PID or service, with one-click **Close App** or **End Task**.
- **Baseline Snapshots & Visual Diffs**: Capture point-in-time system baselines and diff them after installing new software.
- **System & Windows About**: Click the sidebar PC card for pure-Go hardware & OS specs (CPU, RAM speed via SMBIOS, GPU VRAM, local drive bars, motherboard, Device/Product ID, and Windows build).

### 🐕 Watchdog, Tray & Fluent Experience

- **Real-Time Persistence Watchdog**: Watches `Run` / `RunOnce` keys (`RegNotifyChangeKeyValue`) and surfaces native Windows Toast alerts with **Disable Safely** or **Keep** actions.
- **Focus Mode & System Tray**: Temporarily quiet background noise during deep work, or minimize WinBGM to the system tray (`Shell_NotifyIconW`).
- **Thoughtful UX**: Context-aware top commandbar, universal `Esc` back/close navigation, **Dark / Light / AMOLED / System** themes, 6 accent colors, UI scaling (`80%`–`140%`), and built-in **GitHub Releases** update checks.

<details>
<summary><strong>Persistence locations audited by WinBGM</strong></summary>

| Vector                     | Source                                                 | Toggle / Audit Method                      |
| :------------------------- | :----------------------------------------------------- | :----------------------------------------- |
| **User & System Run**      | `HKCU` / `HKLM\...\CurrentVersion\Run`                 | Native `StartupApproved\Run` bitmask       |
| **Startup Folders**        | User & Common Startup folders                          | Native `StartupApproved\StartupFolder`     |
| **Scheduled Tasks**        | Windows Task Scheduler                                 | Native enable / disable + custom delay XML |
| **Windows Services**       | Service Control Manager (`advapi32.dll`)               | Safe demotion to `Manual`                  |
| **Shell Extensions**       | `CLSID\{GUID}\InprocServer32`                          | Non-destructive `Shell Extensions\Blocked` |
| **Browser, Boot & Kernel** | Native Messaging, Active Setup, Winlogon, WMI, Drivers | Inspected & flagged for review             |

</details>

---

## 🛡️ Safety & Guardrails

Every safety boundary is enforced in **compiled Go backend code** before any write occurs:

- **Protected Targets**: WinBGM refuses to modify Windows OS paths (`\Windows\`), critical services (`WinDefend`, `RpcSs`, `DcomLaunch`, `CryptSvc`, etc.), antivirus/security products, kernel processes (`csrss.exe`, `lsass.exe`, `svchost.exe`, `dwm.exe`), shared vendor roots, or WinBGM itself.
- **Pre-Flight Backup & Verification**: Every toggle records its prior state to `%APPDATA%\WinBGM\history.json` **before** applying, verifies the live OS state afterward, and auto-reverts on mismatch.
- **Junk Cleaning Is Always Reversible**: The junk cleaner only moves items to the Recycle Bin (never permanent deletes), re-verifies every path against the in-memory scan cache before touching it, refuses sensitive browser-data paths, and skips files younger than its age thresholds.
- **Package Updates Are Guardrailed**: Winget updates validate the package ID against a strict charset *and* the last scan's cache, spawn winget directly (never through a shell), run one update at a time, and never touch WinBGM itself.
- **System Restore Point**: 1-click native Windows `srclient.dll` restore point creation inside **Settings**.

---

## 📊 How WinBGM Compares

| Capability                        |  Task Manager  | Sysinternals Autoruns  |                    WinBGM                     |
| :-------------------------------- | :------------: | :--------------------: | :-------------------------------------------: |
| **Grouped Startup & Persistence** | Basic Run keys | Ungrouped raw entries  |      **Grouped by App + Delayed Launch**      |
| **Safe Toggle & Rollback**        |    Partial     | Renames / deletes keys |    **`StartupApproved` + `Manual` + Undo**    |
| **OS & Antivirus Guardrails**     |       —        |           —            |        **Go-Enforced Protected Lists**        |
| **Deep Uninstaller & Leftovers**  |       —        |           —            |   **Win32/Store/Steam/Scoop/Choco + Batch**   |
| **Process, EcoQoS & Sockets**     |  Basic EcoQoS  |           —            |   **Grouped CPU/RAM/I/O, Freeze, Sockets**    |
| **Boot, Sleep & File Locks**      |  Basic impact  |           —            | **Event 100–109, `powercfg`, `rstrtmgr.dll`** |
| **Modern Fluent UI**              |  Standard OS   |      Legacy Win32      |      **Dark / Light / AMOLED + Accents**      |

---

## ⌨️ Keyboard Shortcuts

| Shortcut                                      | Action                                     |
| :-------------------------------------------- | :----------------------------------------- |
| <kbd>Ctrl</kbd> + <kbd>K</kbd>                | Focus Global Search                        |
| <kbd>Ctrl</kbd> + <kbd>+</kbd> / <kbd>=</kbd> | Increase UI Scale (+10%)                   |
| <kbd>Ctrl</kbd> + <kbd>-</kbd>                | Decrease UI Scale (-10%)                   |
| <kbd>Ctrl</kbd> + <kbd>0</kbd>                | Reset UI Scale (100%)                      |
| <kbd>Esc</kbd>                                | Universal Back / Close Modals & Inspectors |

---

## 🚀 Quick Start

### Pre-Built Portable Binary

Download the latest portable `.zip` archive from [GitHub Releases](https://github.com/hariharen9/WinBGM/releases), extract `WinBGM.exe`, and run—no installer required.

> **Note:** Because the release binary is not code-signed, Windows SmartScreen may show a prompt on first launch (_More info → Run anyway_).

### Building from Source

```powershell
# Prerequisites: Windows 10/11, Go 1.26+, WebView2 Runtime, and Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

git clone https://github.com/hariharen9/WinBGM.git
cd WinBGM
go mod tidy
wails build -clean -trimpath -o WinBGM.exe
```

Output binary: `build\bin\WinBGM.exe`

---

## 👨‍💻 Creator & License

Created by **[Hariharen](https://hariharen.space)** · [Buy Me a Coffee](https://www.buymeacoffee.com/hariharen)  
Distributed under the **MIT License**. See [LICENSE](LICENSE) for details.
