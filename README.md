<div align="center">

<img src="WinBGM.png" alt="WinBGM Hero Banner" width="100%" style="border-radius: 12px; box-shadow: 0 8px 32px rgba(0,0,0,0.4);" />

# WinBGM (Windows Background Manager)

### *The modern, safe, and transparent background application manager for Windows 10 & 11.*

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Built%20with-Wails%20v2-DF1A29?style=flat&logo=wails)](https://wails.io/)
[![Platform](https://img.shields.io/badge/Platform-Windows%2010%20%7C%2011-0078D6?style=flat&logo=windows)](https://microsoft.com/windows)
[![Architecture](https://img.shields.io/badge/Arch-x64%20%7C%20ARM64%20%7C%20x86-blueviolet?style=flat)]()
[![Safe Mode](https://img.shields.io/badge/Safety-100%25%20Reversible-2ea44f?style=flat&logo=shield)]()
[![VirusTotal](https://img.shields.io/badge/Auditing-VirusTotal%201--Click-3B78FF?style=flat&logo=virustotal)](https://virustotal.com)
[![License](https://img.shields.io/badge/License-MIT-orange?style=flat)](LICENSE)

<br/>

[Key Features](#-key-features) •
[Why WinBGM?](#-why-winbgm-vs-task-manager-vs-autoruns) •
[Safety Architecture](#-safety-architecture-the-8-rules) •
[Persistence Vectors](#-persistence-mechanisms-covered) •
[Boot Profiler & Sleep Blockers](#-boot-profiler--sleep-blocker-detection) •
[Themes & Customization](#-fluent-themes--personalization) •
[Installation & Build](#-installation--building-from-source)

</div>

---

## 💡 Overview

Windows Task Manager is clean, but it only shows basic startup programs and active processes. Sysinternals Autoruns shows everything, but it is an unforgiving power tool with zero safety nets, no rollback history, and a dangerous propensity to brick systems if you uncheck the wrong box.

**WinBGM is the missing middle:**
It exposes everything running behind the scenes—from **Registry Run keys** and **Task Scheduler COM objects** to **Explorer Shell Extensions**, **Browser Native Messaging hosts**, and **WMI subscriptions**—while enforcing an **ironclad, zero-deletion safety engine**. Every change is backed up before execution, verified after execution, and can be restored in a single click.

---

## ⚡ Why WinBGM? (vs. Task Manager vs. Autoruns)

| Capability | Windows Task Manager | Sysinternals Autoruns | WinBGM |
| :--- | :---: | :---: | :---: |
| **Deep Persistence Detection** | Basic (Registry & Startup Folders) | 100+ raw entries (No grouping) | **12 Categorized Vectors (Grouped by App)** |
| **100% Reversible Changes** | Partial | ❌ (Removes keys or marks subkeys) | **✅ Native State Modification (Zero Deletions)** |
| **Atomic Rollback & Audit Trail** | ❌ None | ❌ None | **✅ Full JSON History & 1-Click Undo** |
| **Critical OS / Antivirus Guardrails** | Minimal | ❌ None (Can unboot system) | **✅ 8 Kernel & Service Protection Rules** |
| **Live Deduplicated Background RAM** | ⚠️ Inflated across shared DLLs | ❌ No live telemetry | **✅ Precise `psapi.dll` Deduplicated Working Set** |
| **Boot Event Profiler** | Basic "Impact" label | ❌ None | **✅ Windows Event Log IDs 100-109 Breakdown** |
| **Battery Sleep Blocker Inspector** | ❌ None | ❌ None | **✅ Direct `powercfg /requests` Telemetry** |
| **VirusTotal Hash Verification** | ❌ None | ✅ Hashes only | **✅ 1-Click SHA-256 Lookup Everywhere** |
| **Baseline Snapshots & Differential** | ❌ None | ⚠️ Compare tool (Text diff) | **✅ Rich Visual Snapshot Diffs & Rollbacks** |
| **Process Throttling (Suspend/Freeze)** | ⚠️ Basic End Task | ❌ None | **✅ Freeze, Resume, Graceful Close, Force Kill** |
| **Modern Windows 11 Fluent UI** | Standard OS | Legacy Win32 | **✅ Fluent Design, Dark/Light/AMOLED, Accents** |

---

## ✨ Key Features

### 🛡️ Ironclad Safety Engine
- **Zero Deletions, Ever**: WinBGM never deletes files, scheduled tasks, or registry keys.
  - Startup apps are toggled using the official Windows `StartupApproved` bitmask (identical to Task Manager).
  - Services are demoted to `Manual` start (never `Disabled`), preventing boot loops.
  - Scheduled tasks are toggled via the Windows Task Scheduler COM API (`ITaskService`).
- **Atomic Pre-Flight Backups**: The exact binary state is serialized to `%APPDATA%\WinBGM\history.json` *before* any modification. If backup serialization fails, the modification is aborted immediately.
- **Post-Flight Live Verification**: After writing, the state is read back live. If the system refused the change or deviated from expected bytes, WinBGM rolls back to the original value automatically.

### 🔍 Deep Persistence Discovery (12 Locations)
WinBGM uncovers stealth autostart mechanisms that standard utilities overlook:
1. **Registry Run / RunOnce** (`HKCU` and `HKLM` `Software\Microsoft\Windows\CurrentVersion\Run`)
2. **Startup Folders** (User `%APPDATA%\...\Startup` and Common `%PROGRAMDATA%\...\Startup`)
3. **Windows Services** (Win32 standalone and shared services via `advapi32.dll`)
4. **Scheduled Tasks** (Hidden boot, logon, and recurring maintenance tasks via COM)
5. **Explorer Shell Extensions** (In-process DLL context menu handlers loaded directly into `explorer.exe`)
6. **Browser Native Messaging Hosts** (Persistent IPC bridges for Chrome, Edge, Firefox, and Brave)
7. **Active Setup Components** (Per-user initialization stubs in `Software\Microsoft\Active Setup\Installed Components`)
8. **Winlogon Hooks** (`Userinit` and `Shell` persistence in `Software\Microsoft\Windows NT\CurrentVersion\Winlogon`)
9. **WMI Event Subscriptions** (Persistent WQL `__EventConsumer` and `__FilterToConsumerBinding` instances)
10. **Kernel Drivers & System Services** (Filtered, categorized, and inspected for third-party additions)
11. **Store Apps & Packaged Background Tasks** (UWP and MSIX background task registrations)
12. **App Paths & RunOnceEx** (Auxiliary execution vectors)

### 🧪 VirusTotal & Security Auditing
- **1-Click SHA-256 Lookup**: Check any binary against VirusTotal’s threat database instantly. Available directly across:
  - Scheduled Tasks table & expanded action trays
  - Services inspector & publisher badges
  - Startup Locations table
  - Applications detailed security pane
  - Overview recommendations & action confirmation dialogs
  - Snapshot differential changes
- **System File Impersonation Guard**: Flags deceptive malware attempting to mimic critical Windows binaries (e.g. `svchost.exe`, `csrss.exe`, or `explorer.exe` executed from outside `C:\Windows\System32`).
- **Authenticode Verification**: Queries `wintrust.dll` to verify digital signatures, certificate chains, and trusted publishers.

### ⏱️ Boot Profiler & Sleep Blocker Detection
- **Boot Diagnostics (Event IDs 100-109)**: Pulls data from `Microsoft-Windows-Diagnostics-Performance/Operational` to give an exact millisecond breakdown of:
  - **MainPathBootTime**: Time required to initialize the Windows kernel and reach the user desktop.
  - **PostBootTime**: Time from desktop appearance until the system reaches true idle state.
  - **Degraded Apps & Drivers**: Identifies specific background software delaying boot completion.
- **Sleep Blocker Inspector**: Queries Windows power arbitration (`powercfg /requests`) in real time to show which audio streams, background processes, or display requests are preventing your PC or laptop from entering sleep mode.

### 📸 Baseline Snapshots & Differential Engine
- **State Capture**: Take a snapshot of your system's entire autostart footprint in under a second.
- **Differential Comparison**: Install new software or a game, then diff against your baseline to see exactly what autostart entries, services, or scheduled tasks were injected behind your back.
- **Selective Restoration**: Undo any unauthorized additions with a single click.

### ⏸️ Advanced Process Controls
- **Freeze (Suspend)**: Pause runaway background processes using native thread suspension (`NtSuspendProcess`), freeing 100% of CPU cycles without losing application state or terminating the program.
- **Resume**: Restore frozen processes instantaneously when needed.
- **Graceful Close**: Dispatches standard `WM_CLOSE` signals allowing applications to save unsaved state and shut down cleanly.
- **Force Kill**: Native `TerminateProcess` fallback for uncooperative or hung tasks.

### 🎨 Fluent Themes & Personalization
- **4 Crafted Themes**:
  - 🌙 **Dark**: Refined modern dark theme with layered mica surfaces.
  - ☀️ **Light**: Clean, high-contrast crisp theme for daytime workflows.
  - 🖤 **AMOLED**: True `#000000` pitch black for OLED monitors and battery efficiency.
  - 💻 **System Sync**: Dynamically mirrors your Windows 10/11 system dark/light mode preference.
- **6 Fluent Accent Colors**:
  - 🔷 **Windows Blue** (`#0078D4`)
  - 🟢 **Cyber Emerald** (`#10B981`)
  - 🟣 **Neon Purple** (`#8B5CF6`)
  - 🟠 **Sunset Amber** (`#F59E0B`)
  - 🔴 **Rose Crimson** (`#F43F5E`)
  - 💎 **Electric Cyan** (`#06B6D4`)
- **Dynamic UI Scaling**: Built-in 80% to 140% scaling engine with hotkey support.

---

## 🔒 Safety Architecture (The 8 Rules)

WinBGM's safety checks are enforced entirely in compiled Go backend code. The frontend UI is treated as untrusted and cannot override backend safety assertions.

```
                         [User / UI Action]
                                 │
                                 ▼
                     ┌───────────────────────┐
                     │ 1. Protection Policy  │ ──► Critical Service / AV / WinBGM? ──► [DENY]
                     └───────────────────────┘
                                 │ Pass
                                 ▼
                     ┌───────────────────────┐
                     │ 2. Elevation Check    │ ──► Requires Admin Rights? ──► [PROMPT UAC]
                     └───────────────────────┘
                                 │ Pass
                                 ▼
                     ┌───────────────────────┐
                     │ 3. Live State Read    │ ──► Value changed since scan? ──► [ABORT]
                     └───────────────────────┘
                                 │ Pass
                                 ▼
                     ┌───────────────────────┐
                     │ 4. Pre-Flight Backup  │ ──► Serialize to history.json fails? ──► [ABORT]
                     └───────────────────────┘
                                 │ Success
                                 ▼
                     ┌───────────────────────┐
                     │ 5. Apply Modification │ ──► Native API Write (Manual/Flag/COM)
                     └───────────────────────┘
                                 │
                                 ▼
                     ┌───────────────────────┐
                     │ 6. Post-Flight Verify │ ──► Readback != Expected? ──► [AUTO-RESTORE]
                     └───────────────────────┘
                                 │ Verified
                                 ▼
                       [Operation Complete]
```

1. **Operating System & Security Exclusion**: Core Windows components, critical system services (DCOM, RPCSS, CryptSvc), registered antivirus engines (Windows Defender, CrowdStrike, SentinelOne, Norton, etc.), and WinBGM itself are strictly locked.
2. **Locked Service Protection**: Services configured by the OS with locked access rights are marked read-only; configuration changes are forbidden while stop/start controls remain optional.
3. **Stale Data Invalidation**: Before any modification, the target key or service status is re-read live. If it was modified externally since the last scan, the operation is rejected.
4. **Atomic Pre-Commit History**: The original configuration is serialized and flushed to disk at `%APPDATA%\WinBGM\history.json` before touching the Windows registry or service manager.
5. **Post-Commit Verification**: After applying a change, the state is immediately queried. If the target failed to apply, the previous configuration is automatically reinstated.
6. **Dependency Traversal**: WinBGM refuses to stop any service that has active, running dependent services without explicit user notification.
7. **Strict Exact-Byte Restoration**: Rollbacks only apply if the current system state matches the state recorded when WinBGM made the modification, preventing overwriting manual user adjustments.
8. **Strict UAC Isolation**: Modifications to `HKLM`, Windows Services, or system tasks strictly require administrative privileges. Users can elevate on demand or toggle "Always Start as Administrator".

---

## 🚀 Persistence Mechanisms Covered

| Persistence Vector | Storage / API Path | Method of Deactivation |
| :--- | :--- | :--- |
| **User Run** | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` | `StartupApproved\Run` bitmask toggle (0x02 = Off, 0x01 = On) |
| **System Run** | `HKLM\Software\Microsoft\Windows\CurrentVersion\Run` | `StartupApproved\Run32` bitmask toggle |
| **User Startup** | `%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup` | `StartupApproved\StartupFolder` bitmask toggle |
| **Common Startup** | `%PROGRAMDATA%\Microsoft\Windows\Start Menu\Programs\Startup` | `StartupApproved\StartupFolder` bitmask toggle |
| **Scheduled Tasks** | `TaskScheduler` COM API (`ITaskService`) | Native COM `Task.Enabled = false` |
| **Windows Services** | Windows Service Control Manager (`advapi32.dll`) | `ChangeServiceConfigW` start type -> `SERVICE_DEMAND_START` |
| **Shell Extensions** | `HKLM/HKCU\Software\Classes\CLSID\{GUID}\InprocServer32` | Registry state backup & inspection |
| **Native Messaging** | `Software\Google\Chrome\NativeMessagingHosts` (and Edge/Brave/Firefox) | Manifest inspection & hash validation |
| **Active Setup** | `HKLM\Software\Microsoft\Active Setup\Installed Components` | Stub inspection & binary integrity audit |
| **Winlogon** | `HKLM\Software\Microsoft\Windows NT\CurrentVersion\Winlogon` | Scanned for hijacked `Userinit` / `Shell` entries |
| **WMI Subscriptions** | `root/subscription` (`__EventFilter`, `__EventConsumer`) | Scanned for persistent background scripts |

---

## ⌨️ Keyboard Shortcuts

| Shortcut | Action |
| :--- | :--- |
| <kbd>Ctrl</kbd> + <kbd>K</kbd> | Focus Global Search & Command Palette |
| <kbd>Ctrl</kbd> + <kbd>+</kbd> or <kbd>=</kbd> | Increase UI Scale (+10%) |
| <kbd>Ctrl</kbd> + <kbd>-</kbd> | Decrease UI Scale (-10%) |
| <kbd>Ctrl</kbd> + <kbd>0</kbd> | Reset UI Scale to Default (100%) |
| <kbd>Escape</kbd> | Dismiss Detail Panes, Command Palette, or Dialogs |

---

## 🛠️ Installation & Building from Source

### Download Pre-Built Binaries
Grab the latest standalone `.exe` or `.zip` from the [GitHub Releases](https://github.com/your-username/WinBGM/releases) page:
- **`WinBGM-windows-amd64.exe`**: 64-bit Windows 10 & 11 (Standard Intel/AMD)
- **`WinBGM-windows-arm64.exe`**: 64-bit Windows 11 on ARM (Snapdragon X Elite, Surface Pro, etc.)
- **`WinBGM-windows-386.exe`**: 32-bit legacy Windows

*WinBGM is portable and runs self-contained. No installer required.*

---

### Building from Source

#### Prerequisites
1. **Windows 10 or 11**
2. **Go 1.23+**: [Download Go](https://go.dev/dl/)
3. **Wails v2 CLI**:
   ```powershell
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```
4. **WebView2 Runtime**: Pre-installed on Windows 11 and modern Windows 10 installations (verify using `wails doctor`).
5. **C Compiler (Optional for cgo)**: MinGW-w64 or TDM-GCC if compiling with cgo features.

#### Build Instructions
```powershell
# 1. Clone the repository
git clone https://github.com/your-username/WinBGM.git
cd WinBGM

# 2. Verify dependencies
go mod tidy

# 3. Run in live development mode (hot reload enabled)
wails dev

# 4. Compile optimized production binary
wails build -clean -trimpath -ldflags "-s -w"
```

The production-ready binary will be generated at `build\bin\WinBGM.exe`.

---

## 🏗️ Architecture & Tech Stack

WinBGM pairs a lightning-fast native Go backend with an ultra-lightweight, zero-bloat vanilla frontend:

```
WinBGM/
├── app.go                  # Main Wails application lifecycle & IPC bindings
├── main.go                 # Entry point, single-instance mutex & window config
├── backend/
│   ├── inventory.go        # Master discovery orchestrator & app grouping
│   ├── ops.go              # Atomic change execution & post-write validation
│   ├── protect.go          # The 8 Safety Rules & critical blocklists
│   ├── history.go          # JSON snapshot audit engine & rollback manager
│   ├── admin.go            # Windows UAC elevation detection & restart helpers
│   ├── pe.go               # PE header parsing, SHA256 hashing & Authenticode
│   ├── procs.go            # psapi.dll live memory deduplication & process controls
│   ├── boot.go             # Windows Diagnostics-Performance Event Log parser
│   ├── sleep.go            # powercfg /requests battery blocker parser
│   ├── registry.go         # Direct win32 registry querying & StartupApproved masks
│   ├── services.go         # advapi32.dll Windows Service Control Manager queries
│   ├── tasks.go            # ITaskService COM client for Scheduled Tasks
│   ├── shellext.go         # Explorer Shell Extensions registry scanner
│   ├── nativemessaging.go  # Chrome/Edge/Firefox native host discovery
│   └── wmi.go              # WMI root/subscription persistent consumer query
└── frontend/dist/          # Zero-node modular ES6 frontend
    ├── index.html          # Semantic HTML5 layout
    ├── css/                # Fluent design system (base, layout, components, themes, animations)
    └── js/                 # Modular state machine, components, and controllers
```

- **Backend**: Pure Go utilizing direct Windows Win32 APIs (`psapi.dll`, `advapi32.dll`, `kernel32.dll`, `wintrust.dll`, `ntdll.dll`) and COM bindings (`ole32.dll`, `oleaut32.dll`).
- **Frontend**: Vanilla ES6 modules, CSS Custom Properties, and Lucide icons rendered via Microsoft Edge WebView2. Zero Node runtime overhead, instant boot, and under 30MB of idle memory usage.

---

## 🤝 Contributing

Contributions, feature suggestions, and security audits are welcome!
1. Fork the repository.
2. Create your feature branch (`git checkout -b feature/AmazingFeature`).
3. Ensure all tests pass (`go test ./backend/...`).
4. Commit your changes with descriptive messages.
5. Push to the branch (`git push origin feature/AmazingFeature`).
6. Open a Pull Request.

---

## ⚖️ License & Safety Disclaimer

Distributed under the **MIT License**. See `LICENSE` for details.

> [!NOTE]
> **Safety Guarantee**: WinBGM is engineered with deep defense-in-depth safety checks. However, modifying system services or autostart configurations always carries inherent risks when performed incorrectly. WinBGM prevents destructive operations, but users should always review items before batch modifications.

<div align="center">
  <sub>Crafted with precision for Windows power users and system administrators.</sub>
</div>
