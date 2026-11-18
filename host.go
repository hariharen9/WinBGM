package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func isElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

func osName() string {
	v := windows.RtlGetVersion()
	name := "Windows 10"
	if v.BuildNumber >= 22000 {
		name = "Windows 11"
	}
	return fmt.Sprintf("%s · build %d", name, v.BuildNumber)
}

func hostName() string {
	h, err := os.Hostname()
	if err != nil {
		return "This PC"
	}
	return h
}
