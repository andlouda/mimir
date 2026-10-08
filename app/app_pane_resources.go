package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"mimir/resources"
)

// Per-pane resource overlay: CPU and memory of the process tree below each
// local pane's shell, sampled by the frontend every few seconds in one
// call for all panes. SSH panes have their processes elsewhere and are
// skipped. Read-only; nothing is limited or signalled.

var (
	paneResourceMonitor = resources.NewMonitor()
	tmuxPanePIDs        struct {
		sync.Mutex
		at  time.Time
		pid map[string]int // tmux session name → pane pid
	}
)

const tmuxPanePIDTTL = 60 * time.Second

type paneResourcesResult struct {
	At string `json:"at"`
	// Cores lets the frontend scale the per-core CPU share to the machine.
	Cores int                        `json:"cores"`
	Panes map[string]resources.Usage `json:"panes"`
}

// GetPaneResourcesJSON returns CPU share and memory of the process trees of
// the given local terminals (idsJSON: JSON array of terminal ids).
func (a *App) GetPaneResourcesJSON(idsJSON string) (string, error) {
	var ids []int
	if err := json.Unmarshal([]byte(idsJSON), &ids); err != nil {
		return "", err
	}
	roots := map[string]int{}
	var tmuxNames []string
	for _, id := range ids {
		if a.TerminalManager.GetSSHClient(id) != nil {
			continue
		}
		meta := a.TerminalManager.GetTerminalRuntimeMeta(id)
		if meta.TmuxActive && meta.TmuxSessionName != "" && runtime.GOOS != "windows" {
			tmuxNames = append(tmuxNames, meta.TmuxSessionName)
			continue
		}
		if pid := a.TerminalManager.SessionPID(id); pid > 0 {
			roots[strconv.Itoa(id)] = pid
		}
	}
	if len(tmuxNames) > 0 {
		pids := localTmuxPanePIDs()
		for _, id := range ids {
			meta := a.TerminalManager.GetTerminalRuntimeMeta(id)
			if pid := pids[meta.TmuxSessionName]; meta.TmuxActive && pid > 0 {
				roots[strconv.Itoa(id)] = pid
			}
		}
	}
	res := paneResourcesResult{At: time.Now().Format(time.RFC3339), Cores: runtime.NumCPU(), Panes: map[string]resources.Usage{}}
	if len(roots) > 0 {
		usage, err := paneResourceMonitor.Usage(roots)
		if err != nil {
			return "", err
		}
		res.Panes = usage
	}
	b, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// localTmuxPanePIDs maps every local tmux session to its pane's root pid,
// with one tmux call for all of them, cached for a minute (the pid of a
// pane's shell does not change).
func localTmuxPanePIDs() map[string]int {
	tmuxPanePIDs.Lock()
	defer tmuxPanePIDs.Unlock()
	if tmuxPanePIDs.pid != nil && time.Since(tmuxPanePIDs.at) < tmuxPanePIDTTL {
		return tmuxPanePIDs.pid
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "sh", "-c", `tmux -L mimir list-panes -a -F '#{session_name}	#{pane_pid}' 2>/dev/null`).Output()
	pids := parseTmuxPanePIDs(string(out))
	tmuxPanePIDs.pid, tmuxPanePIDs.at = pids, time.Now()
	return pids
}

func parseTmuxPanePIDs(out string) map[string]int {
	pids := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		name, pidText, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		if pid, err := strconv.Atoi(strings.TrimSpace(pidText)); err == nil && pid > 0 {
			if _, dup := pids[name]; !dup { // first pane of a session is the shell
				pids[name] = pid
			}
		}
	}
	return pids
}
