package main

import (
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func openSCM(access uint32) (*mgr.Mgr, error) {
	h, err := windows.OpenSCManager(nil, nil, access)
	if err != nil {
		return nil, err
	}
	return &mgr.Mgr{Handle: h}, nil
}

// openSvc opens with only the rights requested, so listing works without admin.
func openSvc(m *mgr.Mgr, name string, access uint32) (*mgr.Service, error) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.OpenService(m.Handle, p, access)
	if err != nil {
		return nil, err
	}
	return &mgr.Service{Name: name, Handle: h}, nil
}

// lockedState probes (without changing anything) whether this account may reconfigure the service.
func lockedState(m *mgr.Mgr, name string) string {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return "unknown"
	}
	h, err := windows.OpenService(m.Handle, p, windows.SERVICE_CHANGE_CONFIG)
	if err == nil {
		windows.CloseServiceHandle(h)
		return ""
	}
	if err == windows.ERROR_ACCESS_DENIED {
		return "yes"
	}
	return "unknown"
}

func scanServices(pt procTable, elevated bool) ([]Component, error) {
	m, err := openSCM(windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return nil, err
	}
	defer m.Disconnect()
	names, err := m.ListServices()
	if err != nil {
		return nil, err
	}

	var out []Component
	for _, n := range names {
		s, err := openSvc(m, n, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS)
		if err != nil {
			continue
		}
		cfg, cerr := s.Config()
		st, qerr := s.Query()
		s.Close()
		if cerr != nil || qerr != nil {
			continue
		}
		start, trig := "Manual", "Manual service"
		switch cfg.StartType {
		case mgr.StartAutomatic:
			start, trig = "Automatic", "Automatic service"
			if cfg.DelayedAutoStart {
				start, trig = "Automatic (Delayed)", "Delayed automatic service"
			}
		case mgr.StartDisabled:
			start, trig = "Disabled", "Disabled service"
		}
		c := Component{
			Kind: "service", Name: n, Display: cfg.DisplayName, Command: cfg.BinaryPathName,
			Source: "Services", StartType: start, Trigger: trig,
			Enabled: cfg.StartType != mgr.StartDisabled, AutoStart: cfg.StartType == mgr.StartAutomatic,
			Running: st.State == svc.Running,
		}
		if st.ProcessId != 0 {
			c.PIDs = []uint32{st.ProcessId}
		}
		if elevated && !strings.Contains(strings.ToLower(cfg.BinaryPathName), `\windows\`) {
			c.Locked = lockedState(m, n)
		}
		out = append(out, c)
	}
	return out, nil
}
