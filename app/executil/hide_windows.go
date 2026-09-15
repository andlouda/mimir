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
	// CREATE_NO_WINDOW alone is enough to keep the console from appearing;
	// HideWindow (SW_HIDE via STARTUPINFO) is deliberately not set because the
	// combination looked like process hiding to Defender's heuristics.
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
