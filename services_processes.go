package main

import "winbgm/backend"

// ProcessService exposes live process inspection, EcoQoS, suspension, priority, and termination methods to Wails.
type ProcessService struct {
	app *App
}

// NewProcessService creates a domain service backed by App.
func NewProcessService(app *App) *ProcessService {
	return &ProcessService{app: app}
}

// SuspendProcess freezes all threads of a running process without terminating it.
func (s *ProcessService) SuspendProcess(pid uint32) error {
	return backend.SuspendProcess(pid)
}

// ResumeProcess unfreezes a suspended process.
func (s *ProcessService) ResumeProcess(pid uint32) error {
	return backend.ResumeProcess(pid)
}

// GracefulCloseProcess sends WM_CLOSE to process windows.
func (s *ProcessService) GracefulCloseProcess(pid uint32) error {
	return backend.GracefulCloseProcess(pid)
}

// KillProcess forcefully terminates a process.
func (s *ProcessService) KillProcess(pid uint32) error {
	return backend.KillProcess(pid)
}

// IsProcessSuspended checks if a process was suspended by WinBGM.
func (s *ProcessService) IsProcessSuspended(pid uint32) bool {
	return backend.IsProcessSuspended(pid)
}

// PollProcesses runs an immediate lightweight process & RAM snapshot (<5ms).
func (s *ProcessService) PollProcesses() backend.ProcessPollResult {
	return backend.PollProcesses()
}

// GetLiveProcesses returns all running processes grouped by application alongside CPU/RAM/socket telemetry.
func (s *ProcessService) GetLiveProcesses() LiveProcessesResponse {
	groups, tel := backend.GetLiveProcessGroups()
	return LiveProcessesResponse{Groups: groups, Telemetry: tel}
}

// SetProcessEcoMode toggles Windows 11 Efficiency Mode (EcoQoS) on a single PID.
func (s *ProcessService) SetProcessEcoMode(pid uint32, enable bool) error {
	return backend.SetProcessEcoMode(pid, enable)
}

// SetGroupEcoMode toggles Windows 11 Efficiency Mode (EcoQoS) across all PIDs of an app group.
func (s *ProcessService) SetGroupEcoMode(pids []uint32, enable bool) error {
	var lastErr error
	for _, pid := range pids {
		if err := backend.SetProcessEcoMode(pid, enable); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// SetProcessPriority updates the scheduling priority class of a PID.
func (s *ProcessService) SetProcessPriority(pid uint32, level string) error {
	return backend.SetProcessPriority(pid, level)
}

// KillProcessTree terminates all PIDs belonging to a process group.
func (s *ProcessService) KillProcessTree(pids []uint32) error {
	var killed int
	var lastErr error
	for _, pid := range pids {
		if err := backend.KillProcess(pid); err != nil {
			lastErr = err
		} else {
			killed++
		}
	}
	if killed > 0 {
		return nil
	}
	return lastErr
}
