package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"mimir/agents"
	"mimir/executil"
)

// Process view of an agent: what actually runs under it on the machine
// (test runners, dev servers, sub-agents), read from the process table the
// same way detection does. Nothing is signalled or touched.

type agentProcessesResult struct {
	PID       int                  `json:"pid"`
	Processes []agents.ProcessNode `json:"processes"`
	At        string               `json:"at"`
	Reason    string               `json:"reason,omitempty"`
}

// agentProcessScript lists the process table with elapsed time and CPU
// share through the terminal's tmux host (local, WSL or SSH).
func agentProcessScript(tmuxBin, session string) string {
	return agents.PSDetailedCommand + ` 2>/dev/null`
}

// GetAgentProcessesJSON returns the process tree below the detected agent
// of a terminal.
func (a *App) GetAgentProcessesJSON(terminalID int, terminalType string) (string, error) {
	state, ok := a.rememberedAgent(terminalID)
	if !ok || state.pid <= 0 {
		return "", fmt.Errorf("no agent known for this terminal")
	}
	var (
		procs []agents.Process
		err   error
	)
	if state.source != "ssh" && !a.TerminalManager.GetTerminalRuntimeMeta(terminalID).TmuxActive {
		procs, err = listLocalProcessesDetailed()
	} else {
		var out string
		out, err = a.runPaneScript(terminalID, terminalType, agentProcessScript)
		if err == nil {
			procs = agents.ParsePSDetailed(out)
		}
	}
	if err != nil {
		return "", fmt.Errorf("process list unavailable: %w", err)
	}
	res := agentProcessesResult{PID: state.pid, Processes: agents.Subtree(procs, state.pid), At: time.Now().Format(time.RFC3339)}
	if len(res.Processes) == 0 {
		res.Reason = "agent process not found (exited?)"
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// listLocalProcessesDetailed is listLocalProcesses with elapsed time and CPU
// time. On Windows both come from WMI (CreationDate, kernel+user time).
func listLocalProcessesDetailed() ([]agents.Process, error) {
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
			`$now = Get-Date; Get-CimInstance Win32_Process | ForEach-Object { `+
				`$e = if ($_.CreationDate) { ($now - $_.CreationDate).ToString('hh\:mm\:ss') } else { '-' }; `+
				`$c = [math]::Round((($_.KernelModeTime + $_.UserModeTime) / 10000000), 1); `+
				`"$($_.ProcessId)" + [char]9 + "$($_.ParentProcessId)" + [char]9 + $e + [char]9 + "$($c)s" + [char]9 + "$($_.CommandLine)" }`)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", agents.PSDetailedCommand)
	}
	executil.HideConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return agents.ParsePSDetailed(strings.ReplaceAll(string(out), "\x00", "")), nil
}
