package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"mimir/agents"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// agentStatePayload is emitted as "agent-state-<terminalID>" whenever the
// derived state of an agent changes.
type agentStatePayload struct {
	State       agents.State `json:"state"`
	LastText    string       `json:"lastText,omitempty"`
	LastAt      string       `json:"lastAt,omitempty"`
	SessionFile string       `json:"sessionFile,omitempty"`
}

type agentWatcher struct {
	cancel context.CancelFunc
	kind   agents.Kind
}

const (
	agentWatchLocalInterval  = 2 * time.Second
	agentWatchRemoteInterval = 4 * time.Second
	agentWatchRelookup       = 30 * time.Second
	agentWatchTailBytes      = 64 * 1024
)

// startAgentWatcher follows the agent's session file and pushes state
// changes to the frontend. It replaces title parsing and polling: the file
// is the source of truth for "working" vs "waiting for the user".
func (a *App) startAgentWatcher(terminalID int, terminalType string, state agentTerminalState) {
	desc, ok := agents.Lookup(state.kind)
	if !ok || !desc.Transcripts {
		return
	}
	a.agentMu.Lock()
	if a.agentWatchers == nil {
		a.agentWatchers = make(map[int]*agentWatcher)
	}
	if existing, ok := a.agentWatchers[terminalID]; ok {
		if existing.kind == state.kind {
			a.agentMu.Unlock()
			return
		}
		existing.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.agentWatchers[terminalID] = &agentWatcher{cancel: cancel, kind: state.kind}
	a.agentMu.Unlock()

	go a.watchAgentSession(ctx, terminalID, terminalType, state)
}

func (a *App) stopAgentWatcher(terminalID int) {
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	if w, ok := a.agentWatchers[terminalID]; ok {
		w.cancel()
		delete(a.agentWatchers, terminalID)
	}
}

func (a *App) watchAgentSession(ctx context.Context, terminalID int, terminalType string, state agentTerminalState) {
	fs, home, cleanup, err := a.agentFS(terminalID, state.source)
	if err != nil {
		log.Printf("agent watch %d: %v", terminalID, err)
		return
	}
	defer cleanup()

	interval := agentWatchLocalInterval
	if state.source == "ssh" {
		interval = agentWatchRemoteInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if state.kind == agents.KindOpenCode {
		a.watchOpenCode(ctx, terminalID, state, fs, home, ticker)
		return
	}

	var (
		file        string
		lastLookup  time.Time
		lastSize    int64 = -1
		lastMod     time.Time
		lastPayload agentStatePayload
	)
	locate := func() {
		if state.sessionFile != "" {
			file, lastSize, lastMod = state.sessionFile, -1, time.Time{}
			lastLookup = time.Now()
			return
		}
		cwd := state.cwd
		if cwd == "" {
			cwd = a.TerminalManager.GetLastReportedCwd(terminalID)
		}
		var files []string
		switch state.kind {
		case agents.KindClaude:
			files, _ = agents.FindClaudeTranscripts(fs, home, cwd)
		case agents.KindCodex:
			files, _ = agents.FindCodexTranscripts(fs, home, cwd)
		}
		if len(files) > 0 && files[0] != file {
			file, lastSize, lastMod = files[0], -1, time.Time{}
		}
		lastLookup = time.Now()
	}

	for {
		// Stop when the terminal is gone.
		alive := false
		for _, id := range a.TerminalManager.SessionIDs() {
			if id == terminalID {
				alive = true
				break
			}
		}
		if !alive {
			a.stopAgentWatcher(terminalID)
			return
		}
		if file == "" || time.Since(lastLookup) > agentWatchRelookup {
			locate()
		}
		if file != "" {
			if info, err := fs.Stat(file); err == nil && (info.Size != lastSize || !info.ModTime.Equal(lastMod)) {
				lastSize, lastMod = info.Size, info.ModTime
				if tail, err := fs.ReadTail(file, agentWatchTailBytes); err == nil {
					st := agents.DeriveState(state.kind, tail)
					payload := agentStatePayload{State: st.State, LastText: st.LastText, LastAt: st.LastAt, SessionFile: file}
					if payload != lastPayload {
						lastPayload = payload
						a.emitAgentState(terminalID, payload)
					}
				}
			} else if err != nil {
				file = "" // rotated or deleted: look again on the next tick
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *App) emitAgentState(terminalID int, payload agentStatePayload) {
	if a.ctx == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, fmt.Sprintf("agent-state-%d", terminalID), string(data))
}

// watchOpenCode polls the database's change stamp (main file + WAL) and
// re-derives the state when it moves.
func (a *App) watchOpenCode(ctx context.Context, terminalID int, state agentTerminalState, fs agents.FS, home string, ticker *time.Ticker) {
	dbPath, err := openCodeDBFor(state.source, fs, home)
	if err != nil {
		return
	}
	lastStamp := ""
	var lastPayload agentStatePayload
	for {
		alive := false
		for _, id := range a.TerminalManager.SessionIDs() {
			if id == terminalID {
				alive = true
				break
			}
		}
		if !alive {
			a.stopAgentWatcher(terminalID)
			return
		}
		if stamp := agents.OpenCodeDBStamp(dbPath); stamp != lastStamp {
			lastStamp = stamp
			cwd := state.cwd
			if cwd == "" {
				cwd = a.TerminalManager.GetLastReportedCwd(terminalID)
			}
			sessionID := ""
			if i := strings.LastIndex(state.sessionFile, "#"); i >= 0 {
				sessionID = state.sessionFile[i+1:]
			}
			if st, err := agents.OpenCodeState(dbPath, cwd, sessionID); err == nil {
				payload := agentStatePayload{State: st.State, LastText: st.LastText, LastAt: st.LastAt, SessionFile: dbPath}
				if payload != lastPayload {
					lastPayload = payload
					a.emitAgentState(terminalID, payload)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
