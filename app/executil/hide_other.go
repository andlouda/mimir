//go:build !windows

package executil

import "os/exec"

// HideConsoleWindow is a no-op outside Windows.
func HideConsoleWindow(cmd *exec.Cmd) {}
