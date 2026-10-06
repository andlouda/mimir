package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// tmux keeps its history at the width it was written: after a pane is
// widened, lines scrolled off earlier stay cut at the old width, and xterm
// cannot re-wrap them because tmux emitted hard line breaks. The frontend
// therefore re-fills xterm's scrollback from tmux after a widen:
// GetTmuxHistoryJSON hands over the history with tmux's soft wraps joined
// (-J) and colours kept (-e); RefreshTmuxClient then makes tmux repaint
// the visible screen.

const tmuxHistoryMaxLines = 5000

type tmuxHistoryPayload struct {
	Alternate   bool     `json:"alternate"`
	HistorySize int      `json:"historySize"`
	Lines       []string `json:"lines"`
}

func tmuxHistoryScript(maxLines int) func(tmuxBin, session string) string {
	if maxLines <= 0 || maxLines > tmuxHistoryMaxLines {
		maxLines = tmuxHistoryMaxLines
	}
	return func(tmuxBin, session string) string {
		target := shellQuote(session + ":")
		return tmuxBin + ` display-message -p -t ` + target + ` '#{alternate_on} #{history_size}' 2>/dev/null; ` +
			`echo ` + agentProbeSeparator + `; ` +
			`if [ "$(` + tmuxBin + ` display-message -p -t ` + target + ` '#{alternate_on}' 2>/dev/null)" != 1 ]; then ` +
			tmuxBin + ` capture-pane -p -e -J -S -` + strconv.Itoa(maxLines) + ` -E -1 -t ` + target + ` 2>/dev/null; fi`
	}
}

// tmuxRefreshScript asks every client attached to the session to redraw.
func tmuxRefreshScript(tmuxBin, session string) string {
	q := shellQuote(session)
	return `echo ` + agentProbeSeparator + `; ` +
		tmuxBin + ` list-clients -t ` + q + ` -F '#{client_name}' 2>/dev/null | while read -r c; do ` +
		tmuxBin + ` refresh-client -t "$c" 2>/dev/null; done`
}

// parseTmuxHistory splits the script output into the flags and the lines;
// trailing blank lines are dropped, inner ones kept.
func parseTmuxHistory(output string) tmuxHistoryPayload {
	head, tail, found := strings.Cut(output, agentProbeSeparator)
	if !found {
		return tmuxHistoryPayload{}
	}
	var p tmuxHistoryPayload
	fields := strings.Fields(head)
	if len(fields) > 0 {
		p.Alternate = fields[0] == "1"
	}
	if len(fields) > 1 {
		p.HistorySize, _ = strconv.Atoi(fields[1])
	}
	tail = strings.TrimPrefix(tail, "\n")
	tail = strings.ReplaceAll(tail, "\r\n", "\n")
	lines := strings.Split(tail, "\n")
	// capture-pane pads every row to the pane width; the padding would
	// wrap into blank rows once written at the new width.
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	for len(lines) > 0 && strings.TrimSpace(stripSGR(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	p.Lines = lines
	return p
}

// stripSGR removes colour sequences for the blank-line check only.
func stripSGR(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// GetTmuxHistoryJSON returns the pane's scrollback above the visible
// screen (see the file comment). Alternate-screen panes return no lines.
func (a *App) GetTmuxHistoryJSON(terminalID int, terminalType string, maxLines int) (string, error) {
	output, err := a.runPaneScript(terminalID, terminalType, tmuxHistoryScript(maxLines))
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(parseTmuxHistory(output))
	if err != nil {
		return "", fmt.Errorf("encode tmux history: %w", err)
	}
	return string(data), nil
}

// RefreshTmuxClient makes tmux repaint the pane's visible screen.
func (a *App) RefreshTmuxClient(terminalID int, terminalType string) error {
	_, err := a.runPaneScript(terminalID, terminalType, tmuxRefreshScript)
	return err
}
