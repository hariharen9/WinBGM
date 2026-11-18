package main

import (
	"encoding/json"
	"os/exec"
	"strings"
	"syscall"
)

const psPrelude = `$ProgressPreference='SilentlyContinue'; [Console]::OutputEncoding=[Text.Encoding]::UTF8; `

func ps(script string) ([]byte, error) {
	c := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psPrelude+script)
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

func scanTasks(pt procTable, _ bool) ([]Component, error) {
	raw, err := ps(listTasksPS)
	if err != nil {
		return nil, err
	}
	tasks, err := parseList[psTask](raw)
	if err != nil {
		return nil, err
	}
	var out []Component
	for _, t := range tasks {
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
