package main

import (
	"fmt"
	"strconv"
	"strings"

	"mimir/agentsvc"
	"mimir/aiflow"
	"mimir/executil"
	"mimir/transcript"
)

// Width-independent transcripts for tmux panes; see transcript/history.go.
// The frontend asks for a snapshot while the session lives (before a pane
// is closed, every few minutes while it produces output, and when the
// viewer opens); the viewer then shows the snapshot at one fixed width.

// transcriptHistoryLines bounds one snapshot: the newest lines of tmux's
// history plus the visible screen.
const transcriptHistoryLines = 50000

// transcriptHistoryScript prints the flags line, a separator and the pane's
// history with the visible screen, tmux's soft wraps joined (-J) and no
// colours. A full-screen program (alternate screen) has no history of its
// own; nothing is captured then and the previous snapshot stays.
func transcriptHistoryScript(tmuxBin, session string) string {
	target := executil.ShellQuote(session + ":")
	return tmuxBin + ` display-message -p -t ` + target + ` '#{alternate_on} #{history_size}' 2>/dev/null; ` +
		`echo ` + agentsvc.ProbeSeparator + `; ` +
		`if [ "$(` + tmuxBin + ` display-message -p -t ` + target + ` '#{alternate_on}' 2>/dev/null)" != 1 ]; then ` +
		tmuxBin + ` capture-pane -p -J -S -` + strconv.Itoa(transcriptHistoryLines) + ` -t ` + target + ` 2>/dev/null; fi`
}

// terminalOwnsResumeID reports whether the terminal is the one recorded
// under resumeID, so a snapshot can only land in the pane's own transcript.
func (a *App) terminalOwnsResumeID(terminalID int, resumeID string) bool {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	state, ok := a.activeTerminalStates[terminalID]
	return ok && resumeID != "" && state.ResumeID == resumeID
}

// SnapshotTmuxTranscript saves the tmux history of a pane as the
// width-independent transcript of resumeID. It reports whether a snapshot
// was written; a pane without tmux, a full-screen program or an empty
// history write nothing and are not errors.
func (a *App) SnapshotTmuxTranscript(terminalID int, terminalType string, resumeID string) (bool, error) {
	if !a.terminalOwnsResumeID(terminalID, resumeID) {
		return false, fmt.Errorf("terminal %d does not record transcript %q", terminalID, resumeID)
	}
	output, err := a.agents.RunPaneScript(terminalID, terminalType, transcriptHistoryScript)
	if err != nil {
		// No tmux session behind the pane: nothing to snapshot.
		return false, nil
	}
	history := parseTmuxHistory(output)
	if history.Alternate || len(history.Lines) == 0 {
		return false, nil
	}
	if err := transcript.WriteHistory(resumeID, strings.Join(history.Lines, "\n")+"\n"); err != nil {
		return false, err
	}
	return true, nil
}

// GetTerminalTranscriptHistory returns the width-independent transcript of
// resumeID, empty when the pane never had one (no tmux).
func (a *App) GetTerminalTranscriptHistory(resumeID string, maxBytes int) (transcript.Content, error) {
	if maxBytes <= 0 || maxBytes > transcriptHardCap {
		maxBytes = transcriptHardCap
	}
	return transcript.ReadHistory(resumeID, maxBytes)
}

// GetTerminalTranscriptHistoryScrubbed is GetTerminalTranscriptHistory with
// known secret patterns replaced by [REDACTED], for copy and export.
func (a *App) GetTerminalTranscriptHistoryScrubbed(resumeID string, maxBytes int) (transcript.Content, error) {
	content, err := a.GetTerminalTranscriptHistory(resumeID, maxBytes)
	if err != nil {
		return content, err
	}
	content.Text = aiflow.ScrubSecrets(content.Text)
	return content, nil
}
