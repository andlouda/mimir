package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The Screen tab mirrors the tmux pane in a read-only xterm instead of a
// <pre>: the capture keeps colours and attributes (-e) and tmux's own row
// breaks (no -J), so a TUI (Claude Code's prompt box, spinners, tables)
// looks like it does in the pane and can be scrolled like a terminal.

const screenHistoryLines = 1500

// screenEscapes matches CSI sequences, for the trailing-blank-row check only.
var screenEscapes = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

type agentPaneScreen struct {
	Text      string `json:"text"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Lines     int    `json:"lines"`
	Alternate bool   `json:"alternate"`
}

func agentScreenScript(tmuxBin, session string) string {
	target := shellQuote(session + ":")
	return tmuxBin + ` display-message -p -t ` + target + ` '#{pane_width} #{pane_height} #{alternate_on}' 2>/dev/null; ` +
		`echo ` + agentProbeSeparator + `; ` +
		`if [ "$(` + tmuxBin + ` display-message -p -t ` + target + ` '#{alternate_on}' 2>/dev/null)" = 1 ]; then ` +
		tmuxBin + ` capture-pane -p -e -t ` + target + ` 2>/dev/null; else ` +
		tmuxBin + ` capture-pane -p -e -S -` + strconv.Itoa(screenHistoryLines) + ` -t ` + target + ` 2>/dev/null; fi`
}

// parseAgentScreen keeps the text as tmux printed it (escapes included);
// only trailing blank rows are dropped.
func parseAgentScreen(output string) agentPaneScreen {
	head, tail, found := strings.Cut(output, agentProbeSeparator)
	if !found {
		return agentPaneScreen{}
	}
	var p agentPaneScreen
	fields := strings.Fields(head)
	if len(fields) > 0 {
		p.Width, _ = strconv.Atoi(fields[0])
	}
	if len(fields) > 1 {
		p.Height, _ = strconv.Atoi(fields[1])
	}
	if len(fields) > 2 {
		p.Alternate = fields[2] == "1"
	}
	tail = strings.TrimPrefix(tail, "\n")
	tail = strings.ReplaceAll(tail, "\r\n", "\n")
	lines := strings.Split(tail, "\n")
	for len(lines) > 0 && strings.TrimSpace(screenEscapes.ReplaceAllString(lines[len(lines)-1], "")) == "" {
		lines = lines[:len(lines)-1]
	}
	p.Text = strings.Join(lines, "\n")
	p.Lines = len(lines)
	return p
}

// GetAgentPaneScreenJSON returns the pane's rows with colours for the
// Screen tab's terminal mirror (history included unless the pane runs a
// full-screen program).
func (a *App) GetAgentPaneScreenJSON(terminalID int, terminalType string) (string, error) {
	output, err := a.runPaneScript(terminalID, terminalType, agentScreenScript)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(parseAgentScreen(output))
	if err != nil {
		return "", fmt.Errorf("encode pane screen: %w", err)
	}
	return string(payload), nil
}
