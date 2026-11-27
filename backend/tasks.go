package backend

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

const psPrelude = `$ProgressPreference='SilentlyContinue'; [Console]::OutputEncoding=[Text.Encoding]::UTF8; `

func ps(script string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psPrelude+script)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return c.Output()
}

// parseList decodes PowerShell's ConvertTo-Json output, which is an object for one item and an array for many.
func parseList[T any](raw []byte) ([]T, error) {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[0] == '[' {
		var l []T
		return l, json.Unmarshal(raw, &l)
	}
	var one T
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, err
	}
	return []T{one}, nil
}

type psTask struct {
	Name, Path, State, Triggers, Exec, LastRun string
}

func humanTriggers(raw string) (text string, atStart bool) {
	m := map[string]string{
		"Logon": "At log on", "Boot": "At startup", "Time": "On schedule", "Daily": "On schedule",
		"Weekly": "On schedule", "Monthly": "On schedule", "MonthlyDOW": "On schedule",
		"Event": "On event", "Idle": "When idle", "Registration": "When created",
		"SessionStateChange": "On session change",
	}
	var out []string
	seen := map[string]bool{}
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t == "Logon" || t == "Boot" {
			atStart = true
		}
		h, ok := m[t]
		if !ok {
			if t == "" {
				continue
			}
			h = t
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	if len(out) == 0 {
		return "No trigger", false
	}
	return strings.Join(out, ", "), atStart
}

// scanTasksCOM queries Windows Task Scheduler 2.0 in-process via COM (<50ms).
func scanTasksCOM(pt procTable) ([]Component, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		ole.CoInitialize(0)
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("Schedule.Service")
	if err != nil {
		return nil, err
	}
	defer unknown.Release()

	ts, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, err
	}
	defer ts.Release()

	if _, err := oleutil.CallMethod(ts, "Connect"); err != nil {
		return nil, err
	}

	rootFolderRes, err := oleutil.CallMethod(ts, "GetFolder", `\`)
	if err != nil {
		return nil, err
	}
	rootFolder := rootFolderRes.ToIDispatch()
	defer rootFolder.Release()

	var out []Component

	var scanFolder func(folder *ole.IDispatch, folderPath string)
	scanFolder = func(folder *ole.IDispatch, folderPath string) {
		cleanPath := strings.ToLower(folderPath)
		if strings.HasPrefix(cleanPath, `\microsoft\windows`) {
			return
		}

		// Read tasks in folder
		tasksRes, err := oleutil.CallMethod(folder, "GetTasks", 1) // 1 = TASK_ENUM_HIDDEN
		if err == nil {
			tasksCol := tasksRes.ToIDispatch()
			if countVar, err := oleutil.GetProperty(tasksCol, "Count"); err == nil {
				count := int(countVar.Val)
				for i := 1; i <= count; i++ {
					taskRes, err := oleutil.GetProperty(tasksCol, "Item", i)
					if err != nil {
						continue
					}
					task := taskRes.ToIDispatch()

					nameVar, _ := oleutil.GetProperty(task, "Name")
					name := nameVar.ToString()

					pathVar, _ := oleutil.GetProperty(task, "Path")
					tPath := pathVar.ToString()

					// Extract parent directory of task path for consistency
					dirPath := folderPath
					if !strings.HasSuffix(dirPath, `\`) {
						dirPath += `\`
					}

					stateVar, _ := oleutil.GetProperty(task, "State")
					stateVal := int(stateVar.Val)
					enabled := stateVal != 1   // 1 is TASK_STATE_DISABLED
					isRunning := stateVal == 4 // 4 is TASK_STATE_RUNNING

					lastRunStr := ""
					if lrVar, err := oleutil.GetProperty(task, "LastRunTime"); err == nil {
						if tVal, ok := lrVar.Value().(time.Time); ok && tVal.Year() > 2000 {
							lastRunStr = tVal.Format(time.RFC3339)
						}
					}

					var execParts []string
					var triggerTypes []string

					if defVar, err := oleutil.GetProperty(task, "Definition"); err == nil {
						def := defVar.ToIDispatch()

						// Actions
						if actColVar, err := oleutil.GetProperty(def, "Actions"); err == nil {
							actCol := actColVar.ToIDispatch()
							if aCountVar, err := oleutil.GetProperty(actCol, "Count"); err == nil {
								aCount := int(aCountVar.Val)
								for a := 1; a <= aCount; a++ {
									if aItemVar, err := oleutil.GetProperty(actCol, "Item", a); err == nil {
										aItem := aItemVar.ToIDispatch()
										pVar, _ := oleutil.GetProperty(aItem, "Path")
										argsVar, _ := oleutil.GetProperty(aItem, "Arguments")
										pStr := pVar.ToString()
										argsStr := argsVar.ToString()
										if pStr != "" {
											execParts = append(execParts, strings.TrimSpace(pStr+" "+argsStr))
										}
										aItem.Release()
									}
								}
							}
							actCol.Release()
						}

						// Triggers
						if trigColVar, err := oleutil.GetProperty(def, "Triggers"); err == nil {
							trigCol := trigColVar.ToIDispatch()
							if tCountVar, err := oleutil.GetProperty(trigCol, "Count"); err == nil {
								tCount := int(tCountVar.Val)
								for tr := 1; tr <= tCount; tr++ {
									if tItemVar, err := oleutil.GetProperty(trigCol, "Item", tr); err == nil {
										tItem := tItemVar.ToIDispatch()
										typeVar, _ := oleutil.GetProperty(tItem, "Type")
										switch int(typeVar.Val) {
										case 1:
											triggerTypes = append(triggerTypes, "Event")
										case 2, 3, 4, 5, 6:
											triggerTypes = append(triggerTypes, "Time")
										case 7:
											triggerTypes = append(triggerTypes, "Idle")
										case 8:
											triggerTypes = append(triggerTypes, "Registration")
										case 9:
											triggerTypes = append(triggerTypes, "Boot")
										case 10:
											triggerTypes = append(triggerTypes, "Logon")
										case 11:
											triggerTypes = append(triggerTypes, "SessionStateChange")
										}
										tItem.Release()
									}
								}
							}
							trigCol.Release()
						}
						def.Release()
					}

					execCmd := strings.Join(execParts, " ; ")
					trig, atStart := humanTriggers(strings.Join(triggerTypes, ","))
					exe := exePath(execCmd)
					pids := pt.pidsFor(exe)

					out = append(out, Component{
						Kind:      "task",
						Name:      tPath,
						Display:   name,
						Source:    "Task Scheduler " + dirPath,
						Command:   execCmd,
						ExePath:   exe,
						Trigger:   trig,
						Enabled:   enabled,
						AutoStart: enabled && atStart,
						Running:   isRunning || len(pids) > 0,
						PIDs:      pids,
						LastRun:   lastRunStr,
						tgt:       Target{Kind: "task", Path: dirPath, Name: name},
					})

					task.Release()
				}
			}
			tasksCol.Release()
		}

		// Enumerate subfolders
		subFoldersRes, err := oleutil.CallMethod(folder, "GetFolders", 0)
		if err == nil {
			subFoldersCol := subFoldersRes.ToIDispatch()
			if countVar, err := oleutil.GetProperty(subFoldersCol, "Count"); err == nil {
				count := int(countVar.Val)
				for i := 1; i <= count; i++ {
					subRes, err := oleutil.GetProperty(subFoldersCol, "Item", i)
					if err != nil {
						continue
					}
					sub := subRes.ToIDispatch()
					subPathVar, _ := oleutil.GetProperty(sub, "Path")
					subPath := subPathVar.ToString()
					scanFolder(sub, subPath)
					sub.Release()
				}
			}
			subFoldersCol.Release()
		}
	}

	scanFolder(rootFolder, `\`)
	return out, nil
}

const listTasksPS = `Get-ScheduledTask | Where-Object { $_.TaskPath -notlike '\Microsoft\Windows\*' } | ForEach-Object {
 $t = $_
 $i = Get-ScheduledTaskInfo -TaskName $t.TaskName -TaskPath $t.TaskPath -ErrorAction SilentlyContinue
 $lr = ''
 if ($i -and $i.LastRunTime -and $i.LastRunTime.Year -gt 2000) { $lr = $i.LastRunTime.ToString('o') }
 [pscustomobject]@{
  Name = $t.TaskName; Path = $t.TaskPath; State = [string]$t.State; LastRun = $lr
  Triggers = (($t.Triggers | ForEach-Object { $_.CimClass.CimClassName -replace 'MSFT_Task','' -replace 'Trigger','' }) -join ',')
  Exec = (($t.Actions | ForEach-Object { ($_.Execute + ' ' + $_.Arguments).Trim() }) -join ' ; ')
 } } | ConvertTo-Json -Compress`

func scanTasks(pt procTable, _ bool) ([]Component, error) {
	// Try ultrafast in-process COM first (<50ms)
	tasks, err := scanTasksCOM(pt)
	if err == nil && len(tasks) > 0 {
		return tasks, nil
	}

	// Fallback to PowerShell if COM is unavailable
	raw, err := ps(listTasksPS)
	if err != nil {
		return nil, err
	}
	psTasks, err := parseList[psTask](raw)
	if err != nil {
		return nil, err
	}
	var out []Component
	for _, t := range psTasks {
		trig, atStart := humanTriggers(t.Triggers)
		enabled := t.State != "Disabled"
		exe := exePath(t.Exec)
		pids := pt.pidsFor(exe)
		out = append(out, Component{
			Kind: "task", Name: t.Path + t.Name, Display: t.Name, Source: "Task Scheduler" + " " + t.Path,
			Command: t.Exec, ExePath: exe, Trigger: trig, Enabled: enabled,
			AutoStart: enabled && atStart, Running: t.State == "Running" || len(pids) > 0,
			PIDs: pids, LastRun: t.LastRun,
			tgt: Target{Kind: "task", Path: t.Path, Name: t.Name},
		})
	}
	return out, nil
}
