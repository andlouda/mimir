package executil

import (
	"os/exec"
	"syscall"
)

// HideConsoleWindow keeps a child console process (wsl.exe, tmux, ps, agg)
// from opening a visible console window. Mimir is a GUI-subsystem process,
// so without this flag every helper exec flashes a black window on screen.
func HideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
