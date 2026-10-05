package main

import (
	"encoding/json"
	"log"

	"mimir/session"
)

// Restore bookkeeping.
//
// The saved session is handed to the frontend once (GetLoadedSessionData);
// the frontend then starts the terminals one by one and registers each
// with UpdateTerminalState. Until it reports the outcome through
// FinishSessionRestore, every handed-out entry that has no live terminal
// yet is still written by SaveCurrentSession: a save that fires while the
// restore is running (the first pane's own state update, closing the app
// early, a crash) must not drop the panes that were still to come.
// Entries the frontend could not bring back are kept the same way and
// tried again at the next start.

// sessionStateKey identifies a saved terminal across restarts: the resume
// id when it has one, else the saved shape.
func sessionStateKey(st session.TerminalState) string {
	if st.ResumeID != "" {
		return "r:" + st.ResumeID
	}
	return "s:" + st.Type + "|" + st.Name + "|" + st.SSHProfileID + "|" + st.TmuxSessionName
}

// beginRestore remembers what was handed out and widens the start caps by
// that many terminals: the restore is driven by the saved file, not by a
// runaway frontend loop, so it must not compete with the rate limit.
func (a *App) beginRestore(terminals []session.TerminalState) {
	a.pendingRestore = append([]session.TerminalState(nil), terminals...)
	a.unrestored = nil
	if a.apiLimiter != nil && len(terminals) > 0 {
		a.apiLimiter.grant("start_terminal", len(terminals))
		a.apiLimiter.grant("start_ssh", len(terminals))
	}
}

// retainedTerminalStates returns the handed-out and the unrestored entries
// that no live terminal covers. Called with stateMu held.
func (a *App) retainedTerminalStates() []session.TerminalState {
	live := map[string]bool{}
	for _, st := range a.activeTerminalStates {
		live[sessionStateKey(st)] = true
	}
	var out []session.TerminalState
	seen := map[string]bool{}
	for _, list := range [][]session.TerminalState{a.pendingRestore, a.unrestored} {
		for _, st := range list {
			key := sessionStateKey(st)
			if live[key] || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, st)
		}
	}
	return out
}

// FinishSessionRestore is called by the frontend when the restore loop is
// done. unrestoredJSON lists the saved entries it could not bring back;
// they stay in the session for the next start. Entries whose SSH profile
// no longer exists are dropped, they can never come back.
func (a *App) FinishSessionRestore(unrestoredJSON string) {
	var failed []session.TerminalState
	if unrestoredJSON != "" {
		if err := json.Unmarshal([]byte(unrestoredJSON), &failed); err != nil {
			// Keep everything pending rather than drop it on a bad report.
			log.Printf("session: unreadable restore report: %v", err)
			return
		}
	}
	kept := failed[:0]
	for _, st := range failed {
		if st.Type == "" {
			continue
		}
		if st.Type == "ssh" && st.SSHProfileID != "" && a.sshProfileStore != nil {
			if _, ok := a.sshProfileStore.Get(st.SSHProfileID); !ok {
				continue
			}
		}
		kept = append(kept, st)
	}
	a.stateMu.Lock()
	a.pendingRestore = nil
	a.unrestored = kept
	a.stateMu.Unlock()
	if len(kept) > 0 {
		log.Printf("session: %d terminal(s) not restored, kept for the next start", len(kept))
	}
}

// DiscardUnrestoredTerminal drops one kept entry (the user gave up on it).
func (a *App) DiscardUnrestoredTerminal(key string) {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	kept := a.unrestored[:0]
	for _, st := range a.unrestored {
		if sessionStateKey(st) != key {
			kept = append(kept, st)
		}
	}
	a.unrestored = kept
}
