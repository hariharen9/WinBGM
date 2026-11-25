# WinBGM Review: Quality, Competition and Launch Readiness

*A code review of WinBGM (Windows Background Manager), written from reading the source in the uploaded `WinBGM-main.zip`. The app was not run: it is Windows-only and the review environment was Linux without Go, so every judgment below comes from reading the code, not testing it.*

## Verdict

WinBGM is a seriously ambitious project, but it is not reliable yet. A few problems contradict what the README promises, and they should be fixed before the app is marketed.

## What it is

WinBGM is a portable Windows 10/11 desktop app that finds everything that runs in the background, groups it by application, and lets you disable it safely. The code is about 11.6k lines: a Go backend with a Wails/WebView2 frontend.

- **Autostart discovery:** Run keys, startup folders, scheduled tasks, services, shell extensions, browser native-messaging hosts, drivers, Active Setup, WMI and Winlogon, and AppX apps.
- **Safe toggling:** startup entries use the `StartupApproved` bitmask, services go to Manual instead of Disabled, and every change is backed up to `history.json` first, then read back and reverted if it did not take. That design is good.
- **Extras:**
  - Boot profiler (Event Log IDs 100–109)
  - Sleep-blocker detection
  - Snapshots and diffs
  - A watchdog that alerts when apps re-add themselves
  - Freeze, close and kill process controls, Eco mode and priority
  - A "who is locking this file" tool
  - VirusTotal links
  - A deep uninstaller
  - A system tray icon
  - Exports to JSON, CSV and Markdown

The Go code is cleanly split into modules, and the single-binary approach is smart.

## Does anything like it exist?

Yes, and this is where WinBGM stands:

- **Autoruns** is the gold standard for depth, but it has no guardrails and no undo.
- **Task Manager** is basic.
- **Process Explorer and System Informer** cover process control well.
- **Revo Uninstaller, Bulk Crap Uninstaller and CCleaner** cover uninstall and leftover cleanup. Bulk Crap Uninstaller is open source and very popular.
- **Process Lasso** covers priority and efficiency tuning.

Nobody combines startup management, safe undo, boot profiling, a re-add watchdog and a deep uninstaller in one modern UI. That combination is the angle. No single feature is new, so lean on the combination.

## Why it is not reliable yet

These are real problems, not polish items. Several contradict what the README promises.

1. **"Zero deletions" and "100% reversible" are not true.**
   - The cleanup code (`residuals.go`, `recycle_bin.go`) permanently deletes registry keys and Run values, with no backup. A comment in `SafeDeleteRegistryKey` says it backs up first, but the code does not.
   - If the Recycle Bin move fails, `SafeRecyclePath` silently falls back to `os.RemoveAll`, which is permanent.
2. **The cleanup matching is too broad and pre-selected.**
   - It guesses leftovers from the app name and its first word.
   - An app named "Microsoft …" can match `%LOCALAPPDATA%\Microsoft` and `ProgramData\Microsoft`, and "Google Chrome" can match all of `Software\Google`.
   - A Run-key match uses a substring check on the command line, so a token like "windows" could match unrelated entries.
   - Everything starts selected. This is the most dangerous part of the app.
3. **The UI is trusted in places, even though the code says it never is.**
   - `CleanResidualItems` deletes whatever path and `IsRegistry` flag the frontend sends, with no backend check that the path belongs to the app.
   - Kill, suspend and close in `procs_actions.go` have no protected-process check on the Go side. The `Protected` flag is only used for display.
4. **README claims that do not match the code.**
   - `go.mod` says Go 1.26.0 but the README says 1.23+.
   - There is no LICENSE file in the zip, though the README links to one.
   - The README says downloads are `.exe` files, but the release workflow publishes `.zip` files.
   - The roadmap marks "no PowerShell, all COM" as done, but task listing, enabling and disabling still call PowerShell, and so does the toast notification.
   - "8 hard kernel guardrails" could not be found as a clearly defined set in `protect.go`, so that figure is unverified.
   - Claims like "sub-100ms scans" and "deduplicated RAM" are unverified.
5. **Thin tests.** There are about 24 tests, mostly for parsers (`.lnk`, PE, tasks, history). There are none for the paths that change the system: `Apply`, `Restore` and the cleaner. For a tool that edits services and the registry, those matter most.

## What to do before marketing

- **Fix the cleaner:**
  - Never permanently delete. Fail if the Recycle Bin fails.
  - Export registry keys to `.reg` backups and log them in history.
  - Validate every path on the Go side against an allowlist.
  - Match more strictly, require the match to be in the uninstalled app's own publisher or name folder, and never preselect anything.
- **Move the protected-process check into the Go backend.**
- **Make the README match reality:** drop "100%" and "zero deletions" unless they are true, and add the LICENSE.
- **Add tests for the safety paths**, and test on clean VMs for Windows 10, 11 and ARM.
- **Code-sign the exe** (SignPath has a free tier for open source). Without it, SmartScreen warnings and antivirus false positives are likely, because the app suspends processes, writes to the registry and watches persistence.
- **Add screenshots or a GIF to the README**, and a short "what it touches and what it never touches" document. Trust is the main selling point.

## Is it worth going for stars?

Yes, after the fixes above. Windows utilities do very well on GitHub, especially ones with a polished UI. Good launch channels are r/Windows11, r/sysadmin, Hacker News (Show HN), winget and Scoop, plus a short "why I built a safer Autoruns" post. People will read the code and test the "safe" claims, so one bad uninstall or data-loss report would hurt more than any amount of marketing helps. A sensible plan is to spend the next week or two on the cleaner and the tests, then launch.

## Suggested next steps

- Rework the cleaner and add backend validation.
- Add tests for `Apply`, `Restore` and the cleaner.
- Correct the README, add the LICENSE, and align the release asset names.
- Set up code signing, then launch.
