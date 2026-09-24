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
	// Activity / ActivityAt: the tool call currently running (see
	// agents.StateInfo).
	Activity   string `json:"activity,omitempty"`
	ActivityAt string `json:"activityAt,omitempty"`
	// Prompt is the hook's notification type while one is pending
	// (permission_prompt, idle_prompt, ...); the frontend offers answer
	// buttons only for permission_prompt.
	Prompt string `json:"prompt,omitempty"`
}

type agentWatcher struct {
	cancel context.CancelFunc
	kind   agents.Kind
	pid    int
}

const (
	agentWatchLocalInterval  = 2 * time.Second
	agentWatchRemoteInterval = 4 * time.Second
	agentWatchRelookup       = 30 * time.Second
	agentWatchTailBytes      = 64 * 1024
	// agentHookSettle: how long after a hook event a session-file change
	// still counts as "the record the prompt belongs to".
	agentHookSettle = 3 * time.Second
	// agentHookMaxAge drops a prompt nobody answered (pane closed, Claude
	// Code cancelled) so the badge does not stay red forever.
	agentHookMaxAge = 30 * time.Minute
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
		// Same agent process: keep following. A new process (the user
		// restarted the agent) gets a fresh watcher with the new pid.
		if existing.kind == state.kind && existing.pid == state.pid {
			a.agentMu.Unlock()
			return
		}
		existing.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.agentWatchers[terminalID] = &agentWatcher{cancel: cancel, kind: state.kind, pid: state.pid}
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
	delete(a.agentPrompts, terminalID)
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
		cwd         string
		lastLookup  time.Time
		lastSize    int64 = -1
		lastMod     time.Time
		lastPayload agentStatePayload
		filePayload agentStatePayload
		// Approval hook: the newest notification for this session, kept
		// until the session file moves on (the user answered) or it ages
		// out. Only Claude Code has the hook.
		pending   *agents.HookEvent
		pendingAt time.Time
		// bound is set once a hook event named this pane's session file; the
		// periodic re-lookup then no longer swaps it for the newest file.
		bound bool
	)
	defer a.setAgentPrompt(terminalID, nil, time.Time{})
	eventsDir := ""
	if state.kind == agents.KindClaude {
		eventsDir = hookEventsDir(state.source, fs, home)
	}
	remover, _ := fs.(agents.Remover)
	locate := func() {
		cwd = state.cwd
		if cwd == "" {
			cwd = a.TerminalManager.GetLastReportedCwd(terminalID)
		}
		if state.sessionFile != "" {
			file, lastSize, lastMod = state.sessionFile, -1, time.Time{}
			lastLookup = time.Now()
			return
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
		if file == "" || (!bound && time.Since(lastLookup) > agentWatchRelookup) {
			locate()
		}
		if file != "" {
			if info, err := fs.Stat(file); err == nil && (info.Size != lastSize || !info.ModTime.Equal(lastMod)) {
				first := lastSize < 0
				lastSize, lastMod = info.Size, info.ModTime
				if tail, err := fs.ReadTail(file, agentWatchTailBytes); err == nil {
					st := agents.DeriveState(state.kind, tail)
					filePayload = agentStatePayload{State: st.State, LastText: st.LastText, LastAt: st.LastAt, SessionFile: file, Activity: st.Activity, ActivityAt: st.ActivityAt}
				}
				// The transcript moved on after the prompt: answered. A
				// change right after the event is the tool_use record the
				// prompt belongs to, so give it a moment.
				if !first && pending != nil && time.Since(pendingAt) > agentHookSettle {
					pending = nil
				}
			} else if err != nil {
				file = "" // rotated or deleted: look again on the next tick
			}
		}
		if eventsDir != "" {
			for _, ev := range agents.ScanHookEvents(fs, eventsDir) {
				if !agents.MatchesHookEvent(ev.Event, state.pid, file, cwd) {
					continue
				}
				if remover != nil {
					_ = remover.Remove(ev.Path)
				}
				// The event names the session file of this very process:
				// follow it from now on (unless the user pinned another).
				if state.sessionFile == "" && ev.Event.TranscriptPath != "" && ev.Event.TranscriptPath != file {
					file, lastSize, lastMod, bound = ev.Event.TranscriptPath, -1, time.Time{}, true
				} else if ev.Event.TranscriptPath == file {
					bound = true
				}
				if agents.StateForNotification(ev.Event.NotificationType) == agents.StateUnknown {
					continue
				}
				e := ev.Event
				pending, pendingAt = &e, time.Now()
			}
		}
		if pending != nil && time.Since(pendingAt) > agentHookMaxAge {
			pending = nil
		}
		if pending != nil && pending.NotificationType == "permission_prompt" {
			if _, held := a.heldPrompt(terminalID); !held {
				a.setAgentPrompt(terminalID, pending, pendingAt)
			}
		} else {
			a.setAgentPrompt(terminalID, nil, time.Time{})
		}
		payload := filePayload
		if pending != nil {
			payload.State = agents.StateForNotification(pending.NotificationType)
			payload.LastText = pending.Message
			payload.LastAt = pendingAt.Format(time.RFC3339)
			payload.Prompt = pending.NotificationType
			if payload.SessionFile == "" {
				payload.SessionFile = pending.TranscriptPath
			}
		}
		if payload != lastPayload && payload.State != "" {
			lastPayload = payload
			a.emitAgentState(terminalID, payload)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *App) heldPrompt(terminalID int) (agentPrompt, bool) {
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	p, ok := a.agentPrompts[terminalID]
	return p, ok
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
