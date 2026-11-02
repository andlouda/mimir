//go:build windows

package main

import "golang.org/x/sys/windows"

// windowsBuildNumber is the OS build (22631 for Windows 11 23H2, ...).
func windowsBuildNumber() int {
	return int(windows.RtlGetVersion().BuildNumber)
}
