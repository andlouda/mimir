package main

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"mimir/agents"
)

// Claude Code wraps its finished output at the pane width of the moment,
// with hard line breaks nobody can undo later (anthropics/claude-code
// #8276, #55762 closed as not planned). The one known remedy is a
// restart with --resume: Claude prints the conversation again at the
// current width. RerenderClaudeInPane does exactly that in the agent's
// own pane: /exit, wait for the process to go, run the same command line
// again with --resume <session id>. It is triggered by the user (panel
// link) or, when enabled, after the pane was widened while Claude is
// idle. Nothing is typed while a prompt or an answer is pending.

const (
	rerenderExitTimeout = 12 * time.Second
	rerenderPoll        = 250 * time.Millisecond
	rerenderShellSettle = 400 * time.Millisecond
)

// claudeResumeCommand rebuilds the agent's command line with the session
// to resume, replacing an earlier --resume/-r/--continue/-c.
func claudeResumeCommand(args string, sessionID string) (string, error) {
	tokens := agents.SplitCommandLine(args)
	if len(tokens) == 0 {
		return "", errors.New("agent command line unknown")
	}
	var kept []string
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch {
		case t == "--resume" || t == "-r":
			if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "-") {
				i++
			}
			continue
		case strings.HasPrefix(t, "--resume="):
			continue
		case t == "--continue" || t == "-c":
			continue
		}
		kept = append(kept, t)
	}
	quoted := make([]string, 0, len(kept)+2)
	for _, t := range kept {
		quoted = append(quoted, shellQuoteIfNeeded(t))
	}
	quoted = append(quoted, "--resume", shellQuoteIfNeeded(sessionID))
	return strings.Join(quoted, " "), nil
}

func shellQuoteIfNeeded(t string) string {
	if t == "" {
		return "''"
	}
	for _, r := range t {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:@%+,", r)) {
			return shellQuote(t)
		}
	}
	return t
}

func claudeSessionID(state agentTerminalState) string {
	file := state.effectiveSessionFile()
	if file == "" {
		return ""
	}
	base := filepath.Base(strings.ReplaceAll(file, "\\", "/"))
	return strings.TrimSuffix(base, ".jsonl")
}

// resolveClaudeSessionFile picks the session file for a pane the hook has
// not bound: the sole candidate for its directory, or the candidate the
// pane text verifies.
func (a *App) resolveClaudeSessionFile(terminalID int, terminalType string, state agentTerminalState) (string, error) {
	paneText, _ := a.capturePane(terminalID, terminalType)
	transcript, err := a.readAgentTranscriptFile(terminalID, state, agents.ReadOptions{Limit: 4, PaneText: paneText})
	if err != nil {
		return "", fmt.Errorf("the session of this pane is not known yet (%v); it is bound once Claude Code's hook reports the next prompt", err)
	}
	if transcript.SessionFile == "" {
		return "", errors.New("the session of this pane is not known yet; it is bound once Claude Code's hook reports the next prompt")
	}
	if transcript.Verified || transcript.Candidates <= 1 {
		return transcript.SessionFile, nil
	}
	return "", fmt.Errorf("%d Claude sessions exist for this directory and none could be matched to the pane; install the approval hook or pin the session in the panel", transcript.Candidates)
}

// agentCommandLine returns the agent process's own command line.
func (a *App) agentCommandLine(terminalID int, terminalType string, state agentTerminalState) (string, error) {
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
	for _, p := range procs {
		if p.PID == state.pid {
			return p.Args, nil
		}
	}
	return "", errors.New("agent process not found")
}

func (a *App) agentProcessAlive(terminalID int, terminalType string, state agentTerminalState) bool {
	_, err := a.agentCommandLine(terminalID, terminalType, state)
	return err == nil
}

// RerenderClaudeInPane restarts Claude Code in the pane with --resume so
// its output is printed again at the current width. Returns the command
// that was typed.
func (a *App) RerenderClaudeInPane(terminalID int, terminalType string) (string, error) {
	state, ok := a.rememberedAgent(terminalID)
	if !ok || state.kind != agents.KindClaude || state.pid <= 0 {
		return "", errors.New("no Claude Code process known in this pane")
	}
	if _, held := a.heldPrompt(terminalID); held {
		return "", errors.New("Claude is waiting for an approval; answer it first")
	}
	sessionID := claudeSessionID(state)
	if sessionID == "" {
		// Not bound by the hook yet: the directory's only session, or the
		// one whose last answer is on the pane's screen, is still a safe
		// pick; several unverifiable candidates are not.
		file, err := a.resolveClaudeSessionFile(terminalID, terminalType, state)
		if err != nil {
			return "", err
		}
		sessionID = claudeSessionID(agentTerminalState{sessionFile: file})
	}
	args, err := a.agentCommandLine(terminalID, terminalType, state)
	if err != nil {
		return "", err
	}
	command, err := claudeResumeCommand(args, sessionID)
	if err != nil {
		return "", err
	}
	// Leave Claude through its own command; nothing else is typed until the
	// process is gone, so a late keystroke cannot land in a prompt.
	if err := a.TerminalManager.WriteToTerminal(terminalID, "/exit\r"); err != nil {
		return "", err
	}
	deadline := time.Now().Add(rerenderExitTimeout)
	for a.agentProcessAlive(terminalID, terminalType, state) {
		if time.Now().After(deadline) {
			return "", errors.New("Claude did not exit; nothing restarted")
		}
		time.Sleep(rerenderPoll)
	}
	time.Sleep(rerenderShellSettle)
	if err := a.TerminalManager.WriteToTerminal(terminalID, command+"\r"); err != nil {
		return "", err
	}
	a.stopAgentWatcher(terminalID) // the new process gets its own watcher on the next detection
	log.Printf("agent rerender: terminal %d: %s", terminalID, command)
	logAgentEvent("agent_rerender", fmt.Sprintf("terminal %d: %s", terminalID, command))
	return command, nil
}
