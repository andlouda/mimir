//go:build !windows

package main

// windowsBuildNumber is 0 off Windows.
func windowsBuildNumber() int { return 0 }
