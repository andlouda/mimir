//go:build !windows

package recording

import "os/exec"

func hideConsoleWindow(cmd *exec.Cmd) {}
