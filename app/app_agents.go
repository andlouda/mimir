package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mimir/executil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"mimir/activitylog"
	"mimir/agents"
)

// agentDetectionResult is returned to the frontend by DetectAgentForTerminalJSON.
type agentDetectionResult struct {
	Detected    bool        `json:"detected"`
	Kind        agents.Kind `json:"kind,omitempty"`
	Label       string      `json:"label,omitempty"`
	PID         int         `json:"pid,omitempty"`
	Cwd         string      `json:"cwd,omitempty"`
	Source      string      `json:"source,omitempty"` // local | wsl | ssh
	Transcripts bool        `json:"transcripts"`
	// Reason explains why nothing could be detected (no tmux, unsupported
	// terminal type); it is informational, not an error.
	Reason string `json:"reason,omitempty"`
}

// agentTerminalState caches the last successful detection per terminal so the
// transcript request does not have to re-scan the process tree.
type agentTerminalState struct {
	kind   agents.Kind
	cwd    string
	source string
}

var tmuxSessionNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

const agentProbeSeparator = "__MIMIR_PS__"

// agentProbeScript prints the pane's root pid and cwd, then the process list.
// tmuxBin is the tmux invocation (with socket flag for local sessions).
func agentProbeScript(tmuxBin, session string) string {
	target := shellQuote(session + ":")
	return tmuxBin + ` display-message -p -t ` + target + ` '#{pane_pid}` + "\t" + `#{pane_current_path}' 2>/dev/null; ` +
		`echo ` + agentProbeSeparator + `; ` + agents.PSCommand + ` 2>/dev/null`
}

// paneCaptureLines bounds how much scrollback the tmux capture returns.
const paneCaptureLines = 1500

// agentCaptureScript prints the pane width, a separator and the pane's
// visible text plus recent scrollback. -J re-joins tmux's own soft wraps.
func agentCaptureScript(tmuxBin, session string) string {
	target := shellQuote(session + ":")
	return tmuxBin + ` display-message -p -t ` + target + ` '#{pane_width}' 2>/dev/null; ` +
		`echo ` + agentProbeSeparator + `; ` +
		tmuxBin + ` capture-pane -p -J -S -` + strconv.Itoa(paneCaptureLines) + ` -t ` + target + ` 2>/dev/null`
}

// parseAgentCapture splits capture output into pane width and cleaned text
// with hard-wrapped full-width rows re-joined.
func parseAgentCapture(output string) (text string, width int) {
	head, tail, found := strings.Cut(output, agentProbeSeparator)
	if !found {
		return "", 0
	}
	width, _ = strconv.Atoi(strings.TrimSpace(head))
	text = agents.CleanPaneText(strings.TrimPrefix(tail, "\n"))
	return agents.JoinFullWidthRows(text, width), width
}

// runPaneScript executes a tmux script in the environment the terminal
// lives in (local shell, WSL distro or SSH host).
func (a *App) runPaneScript(terminalID int, terminalType string, build func(tmuxBin, session string) string) (string, error) {
	if client := a.TerminalManager.GetSSHClient(terminalID); client != nil {
		session := ""
		if meta := a.TerminalManager.GetSSHMeta(terminalID); meta != nil {
			session = strings.TrimSpace(meta.Config.TmuxSessionName)
		}
		if session == "" || !tmuxSessionNamePattern.MatchString(session) {
			return "", fmt.Errorf("no tmux session on the remote host")
		}
		output, err := runSSHCommandWithTimeout(client, build("tmux", session), discoveryTimeout)
		if err != nil && !strings.Contains(output, agentProbeSeparator) {
			return "", fmt.Errorf("remote tmux probe failed")
		}
		return output, nil
	}
	meta := a.TerminalManager.GetTerminalRuntimeMeta(terminalID)
	session := strings.TrimSpace(meta.TmuxSessionName)
	if !meta.TmuxActive || session == "" || !tmuxSessionNamePattern.MatchString(session) {
		return "", fmt.Errorf("no tmux session")
	}
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()
	var cmd *exec.Cmd
	switch strings.ToLower(strings.TrimSpace(terminalType)) {
	case "wsl":
		cmd = exec.CommandContext(ctx, "wsl.exe", "--", "sh", "-c", build("tmux -L mimir", session))
	case "bash", "zsh":
		cmd = exec.CommandContext(ctx, "sh", "-c", build("tmux -L mimir", session))
	default:
		return "", fmt.Errorf("agent features need a tmux-backed terminal")
	}
	executil.HideConsoleWindow(cmd)
	output, err := cmd.Output()
	if err != nil && !strings.Contains(string(output), agentProbeSeparator) {
		return "", fmt.Errorf("local tmux probe failed")
	}
	return string(output), nil
}

// capturePane returns the pane text (see agentCaptureScript) or "" on error.
func (a *App) capturePane(terminalID int, terminalType string) (string, int) {
	output, err := a.runPaneScript(terminalID, terminalType, agentCaptureScript)
	if err != nil {
		return "", 0
	}
	return parseAgentCapture(output)
}

// agentPaneText is the payload of GetAgentPaneTextJSON.
type agentPaneText struct {
	Text  string `json:"text"`
	Width int    `json:"width"`
	Lines int    `json:"lines"`
}

// GetAgentPaneTextJSON returns the tmux pane contents of a terminal (visible
// screen plus recent scrollback) with soft wraps joined. It works for every
// tmux-backed terminal regardless of which agent runs in it and does not
// depend on any agent's file format; the trade-off is that it contains
// whatever the agent drew, including its own line breaks.
func (a *App) GetAgentPaneTextJSON(terminalID int, terminalType string) (string, error) {
	output, err := a.runPaneScript(terminalID, terminalType, agentCaptureScript)
	if err != nil {
		return "", err
	}
	text, width := parseAgentCapture(output)
	logAgentEvent("agent_pane_read", fmt.Sprintf("%d lines", strings.Count(text, "\n")+1))
	payload, err := json.Marshal(agentPaneText{Text: text, Width: width, Lines: strings.Count(text, "\n") + 1})
	if err != nil {
		return "", fmt.Errorf("failed to encode pane text: %w", err)
	}
	return string(payload), nil
}

// parseAgentProbe splits probe output into the pane root pid, its cwd and the
// process rows.
func parseAgentProbe(output string) (rootPID int, cwd string, procs []agents.Process) {
	head, tail, found := strings.Cut(output, agentProbeSeparator)
	if !found {
		return 0, "", nil
	}
	line := strings.TrimSpace(head)
	if line != "" {
		pidStr, dir, _ := strings.Cut(line, "\t")
		rootPID, _ = strconv.Atoi(strings.TrimSpace(pidStr))
		cwd = strings.TrimSpace(dir)
	}
	return rootPID, cwd, agents.ParsePS(tail)
}

func (a *App) rememberAgent(terminalID int, state agentTerminalState) {
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	if a.agentStates == nil {
		a.agentStates = make(map[int]agentTerminalState)
	}
	a.agentStates[terminalID] = state
}

func (a *App) forgetAgent(terminalID int) {
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	delete(a.agentStates, terminalID)
}

func (a *App) rememberedAgent(terminalID int) (agentTerminalState, bool) {
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	s, ok := a.agentStates[terminalID]
	return s, ok
}

// DetectAgentForTerminalJSON scans the process tree of a terminal's tmux pane
// for a known coding agent. Detection is metadata only: the process list is
// inspected in memory and never shown; the interactive PTY is not touched.
func (a *App) DetectAgentForTerminalJSON(terminalID int, terminalType string) (string, error) {
	result := a.detectAgent(terminalID, terminalType)
	if result.Detected {
		a.rememberAgent(terminalID, agentTerminalState{kind: result.Kind, cwd: result.Cwd, source: result.Source})
	} else {
		a.forgetAgent(terminalID)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to encode agent detection: %w", err)
	}
	return string(payload), nil
}

func (a *App) detectAgent(terminalID int, terminalType string) agentDetectionResult {
	source := "local"
	if a.TerminalManager.GetSSHClient(terminalID) != nil {
		source = "ssh"
	} else if strings.EqualFold(strings.TrimSpace(terminalType), "wsl") {
		source = "wsl"
	}
	output, err := a.runPaneScript(terminalID, terminalType, agentProbeScript)
	if err != nil {
		return agentDetectionResult{Reason: err.Error()}
	}
	return finishDetection(output, source)
}

func finishDetection(output, source string) agentDetectionResult {
	rootPID, cwd, procs := parseAgentProbe(output)
	if rootPID <= 0 {
		return agentDetectionResult{Reason: "tmux pane not found"}
	}
	det, ok := agents.FindAgent(procs, rootPID)
	if !ok {
		return agentDetectionResult{Cwd: cwd, Source: source}
	}
	return agentDetectionResult{
		Detected:    true,
		Kind:        det.Kind,
		Label:       det.Label,
		PID:         det.PID,
		Cwd:         cwd,
		Source:      source,
		Transcripts: det.Transcripts,
	}
}

// GetAgentTranscriptJSON returns the last messages of the agent session that
// belongs to a terminal. The session file is read out-of-band from the agent's
// own storage (never through the PTY), so the returned text is exact and free
// of terminal wrapping. Only metadata is logged.
func (a *App) GetAgentTranscriptJSON(terminalID int, terminalType string, limit int) (string, error) {
	state, ok := a.rememberedAgent(terminalID)
	if !ok {
		// Detection may not have run yet (panel opened directly); run it now.
		res := a.detectAgent(terminalID, terminalType)
		if !res.Detected {
			return "", fmt.Errorf("no agent detected in this terminal")
		}
		state = agentTerminalState{kind: res.Kind, cwd: res.Cwd, source: res.Source}
		a.rememberAgent(terminalID, state)
	}
	if state.cwd == "" {
		return "", fmt.Errorf("could not resolve the agent's working directory")
	}

	paneText, _ := a.capturePane(terminalID, terminalType)

	transcript, err := a.readAgentTranscriptFile(terminalID, state, agents.ReadOptions{Limit: limit, PaneText: paneText})
	if err != nil {
		// tmux is always there: fall back to the pane contents when the
		// agent has no readable session file (unsupported agent, changed
		// format, history elsewhere).
		if paneText == "" {
			return "", fmt.Errorf("read agent transcript: %w", err)
		}
		label := ""
		if d, ok := agents.Lookup(state.kind); ok {
			label = d.Label
		}
		transcript = agents.PaneTranscript(state.kind, label, state.cwd, paneText)
		logAgentEvent("agent_transcript_fallback", fmt.Sprintf("%s via %s: pane capture (%v)", state.kind, state.source, err))
	} else {
		logAgentEvent("agent_transcript_read", fmt.Sprintf("%s via %s: %d messages, verified=%v", state.kind, state.source, len(transcript.Messages), transcript.Verified))
	}

	payload, err := json.Marshal(transcript)
	if err != nil {
		return "", fmt.Errorf("failed to encode agent transcript: %w", err)
	}
	return string(payload), nil
}

func (a *App) readAgentTranscriptFile(terminalID int, state agentTerminalState, opts agents.ReadOptions) (agents.Transcript, error) {
	fs, home, cleanup, err := a.agentFS(terminalID, state.source)
	if err != nil {
		return agents.Transcript{}, err
	}
	defer cleanup()
	transcript, err := agents.ReadTranscript(fs, state.kind, home, state.cwd, opts)
	if err != nil {
		if errors.Is(err, agents.ErrNotFound) {
			return agents.Transcript{}, fmt.Errorf("no %s session file found for %s", state.kind, state.cwd)
		}
		return agents.Transcript{}, err
	}
	return transcript, nil
}

// agentFS returns the filesystem and home directory where the agent stores
// its sessions, depending on where the terminal runs.
func (a *App) agentFS(terminalID int, source string) (agents.FS, string, func(), error) {
	noop := func() {}
	switch source {
	case "ssh":
		client, err := a.remoteFileClient(terminalID)
		if err != nil {
			return nil, "", noop, err
		}
		home, err := client.Getwd()
		if err != nil || home == "" {
			client.Close()
			return nil, "", noop, fmt.Errorf("could not resolve remote home directory")
		}
		return sftpAgentFS{client: client}, home, func() { client.Close() }, nil
	case "wsl":
		base, home, err := wslHomeUNC()
		if err != nil {
			return nil, "", noop, err
		}
		return agents.LocalFS{Base: base}, home, noop, nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", noop, fmt.Errorf("could not resolve home directory: %w", err)
		}
		return agents.LocalFS{}, home, noop, nil
	}
}

// wslHomeUNC resolves the default WSL distro's home directory as a UNC base
// (\\wsl$\<distro>) plus the Linux home path, so the Windows-side process can
// read the agent's session files without another exec per file.
func wslHomeUNC() (base string, home string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", "--", "sh", "-c", `printf '%s\n%s\n' "$HOME" "$WSL_DISTRO_NAME"`)
	executil.HideConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("could not query WSL home: %s", firstOutputLine(strings.TrimSpace(string(out))))
	}
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(out), "\x00", "")), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" {
		return "", "", fmt.Errorf("could not query WSL home")
	}
	return `\\wsl$\` + strings.TrimSpace(lines[1]), strings.TrimSpace(lines[0]), nil
}

// sftpAgentFS adapts the SFTP client to agents.FS.
type sftpAgentFS struct {
	client remoteFileClient
}

func (s sftpAgentFS) Join(elem ...string) string { return path.Join(elem...) }

func (s sftpAgentFS) ReadDir(dir string) ([]agents.FileInfo, error) {
	entries, err := s.client.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]agents.FileInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, agents.FileInfo{Name: e.Name(), IsDir: e.IsDir(), ModTime: e.ModTime(), Size: e.Size()})
	}
	return out, nil
}

func (s sftpAgentFS) ReadHead(file string, max int64) ([]byte, error) {
	f, err := s.client.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}

func (s sftpAgentFS) ReadTail(file string, max int64) ([]byte, error) {
	f, err := s.client.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return agents.ReadTailFrom(f, max)
}

func logAgentEvent(event, reason string) {
	_ = activitylog.Append(activitylog.KindSecurityEvents, activitylog.SecurityEventEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     event,
		Operation: "agent_panel",
		Reason:    reason,
	})
}
