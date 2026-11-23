# WinBGM Development Roadmap & Master TODO

A prioritized, phased roadmap to evolve WinBGM into the premier, safe, modern alternative to Microsoft Sysinternals Autoruns.

---

## 🏗️ Phase 0: Project Architecture & Code Structure
- [x] **Clean Project Restructuring**:
  - [x] Move backend domain logic out of root into clean, modular `backend/` package.
  - [x] Keep root directory minimal and organized (`app.go`, `main.go`, `go.mod`, `wails.json`, assets).
  - [x] Verify `wails build` and TS bindings compile cleanly in 6.0s.

---

## ⚡ Phase 1: High-Performance Engine & Core Strengthening
*Goal: Eliminate external CLI bottlenecks, achieve sub-100ms scans, and deepen existing safety checks.*

- [x] **1.1: Native In-Process Shortcut (.lnk) Parser**
  - Implement binary MS-SHLLINK parser in pure Go.
  - Eliminate PowerShell COM `WScript.Shell` invocation in `scanFolders`.
  - Cut shortcut scan latency from ~600ms down to <2ms.

- [x] **1.2: Task Scheduler 2.0 COM Integration**
  - Connect directly to `CLSID_TaskScheduler` via COM (`ITaskService`).
  - Query scheduled task definitions, actions, triggers, and state in-process.
  - Eliminate `powershell.exe Get-ScheduledTask` subprocess overhead.

- [x] **1.3: Windows Catalog File Signature Verification (`pe.go`)**
  - Integrate `CryptCATAdminAcquireContext` and `CryptCATAdminEnumCatalogFromHash`.
  - Accurately verify catalog-signed Windows system files (`.cat` in `CatRoot`) as `verified` rather than `unsigned`.

- [x] **1.4: Detailed Certificate Signer & Subject Identity**
  - Extract and display the actual certificate subject name (e.g., *"Signed by Google LLC"* vs *"Signed by Valve Corporation"*).
  - Detect and flag expired or revoked certificates.

- [x] **1.5: Lightweight Real-Time Background Process Polling**
  - Add non-blocking 3-second background ticker checking only active PIDs and RAM (without disk/registry rescans).
  - Update running states and memory badges live with smooth UI transitions.

- [x] **1.6: Batch Session Rollback in History**
  - Group batch changes (e.g., Focus Mode sessions) under a shared `BatchID`.
  - Add a 1-click "Restore Entire Session" button in Change History.

---

## ⏱️ Phase 2: Boot & Sleep Performance Profiler (The Killer Features)
*Goal: Show users real metrics on what delays their PC's startup and what keeps it awake.*

- [x] **2.1: Windows Event Log Boot Diagnostic Reader**
  - Query Event Log channel `Microsoft-Windows-Diagnostics-Performance/Operational`.
  - Parse Event ID `100` (Boot Duration) and Event IDs `101-109` (Boot Degradation by App/Service).
  - Display exact startup delay in seconds/milliseconds per app (e.g., *"Docker: +4.2s delay at boot"*).

- [x] **2.2: Power & Sleep Blocker Scanner (`powercfg /requests`)**
  - Query active Windows Power Requests via Win32 API (`PowerReadRequestList` / `powercfg`).
  - Flag background apps and drivers that prevent Windows from entering sleep or display idle.
  - Show a "Sleep Blockers" indicator with explanation.

---

## 🔍 Phase 3: New Persistence & Extensibility Vectors
*Goal: Equal and surpass Autoruns in coverage of common real-world clutter and stealth autostarts.*

- [x] **3.1: Explorer Shell Extensions & Context Menu Scanner**
  - Scan `HKCR\*\shellex\ContextMenuHandlers`, `Directory\shellex`, and `Folder\shellex`.
  - Identify third-party context menu clutter loaded into `explorer.exe` (e.g. 7-Zip, WinRAR, Git, Dropbox, PowerToys).
  - Add safe toggle to enable/disable sluggish shell extensions via `Shell Extensions\Blocked`.

- [x] **3.2: Native Messaging Hosts Scanner**
  - Scan `HKCU`/`HKLM\Software\Google\Chrome\NativeMessagingHosts` (plus Edge, Brave, Firefox).
  - List desktop background binaries registered to communicate directly with browser extensions.

- [x] **3.3: Third-Party Kernel Drivers & Peripheral Services**
  - Enumerate third-party non-Microsoft kernel drivers from `SYSTEM\CurrentControlSet\Services` (Type 1 and 2).
  - Categorize anti-cheat drivers (Vanguard, EasyAntiCheat, BattlEye) and peripheral software (Razer, Corsair, Logitech).

- [x] **3.4: Active Setup & Logon Scripts Scanner**
  - Scan `HKLM\SOFTWARE\Microsoft\Active Setup\Installed Components` and 32-bit `Wow6432Node`.
  - Detect stubborn per-user initialization stubs executing before shell load.

---

## 🛡️ Phase 4: Active Protection & App Management
*Goal: Transform WinBGM into an active assistant that protects your system over time.*

- [x] **4.1: Background Persistence Watcher ("Re-Add Sentinel")**
  - Background registry watcher via `RegNotifyChangeKeyValue` on `Run` and `RunOnce` keys.
  - Automatically emits real-time alert notifications when apps (Discord, Spotify, Teams, Chrome) silently re-add themselves at boot.

- [x] **4.2: Snapshot & Diff Comparison ("What Changed?")**
  - Allow saving named baseline snapshots ("Clean Install", "Before installing Game").
  - Provide visual Diff viewer highlighting all new startup items, tasks, and services installed since baseline.

- [x] **4.3: Interactive Process Management (Graceful End & Suspend)**
  - Add "Graceful Close" (`WM_CLOSE` to top-level windows).
  - Add "Freeze / Suspend" (`NtSuspendProcess` via `ntdll.dll`) and "Resume" (`NtResumeProcess`) to pause resource-heavy background processes without killing them.
  - Provide forceful terminate fallback option.

- [x] **4.4: AppX/MSIX Manifest Deep Metadata**
  - Parse `AppxManifest.xml` directly under `WindowsApps` to extract official human-readable display names, version, and publishers.
