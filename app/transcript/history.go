package transcript

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"mimir/safeio"
)

// A history snapshot is the transcript of a tmux pane as logical lines.
//
// The .log file records the raw byte stream. For a tmux pane that stream is
// tmux painting the screen at whatever width the pane had at the time, and
// painting it again after every resize: the text is cut at changing widths
// and repeated. tmux's own history knows which rows belong to one line, so
// a snapshot of it ("capture-pane -J") is independent of any pane width and
// can be shown at one fixed width for good.
//
// The snapshot lives next to the log as <resumeID>.history and is replaced
// as a whole each time; it never grows by appending.

// MaxHistorySize bounds one snapshot on disk.
const MaxHistorySize = 16 * 1024 * 1024

func historyPath(resumeID string) (string, error) {
	logPath, err := PathForResumeID(resumeID)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(logPath, ".log") + ".history", nil
}

// WriteHistory replaces the history snapshot of a transcript. Text beyond
// MaxHistorySize is cut from the front (the newest lines are kept), at a
// line boundary. An empty text leaves an existing snapshot in place: a
// session that is gone must not wipe what was saved while it lived.
func WriteHistory(resumeID string, text string) error {
	path, err := historyPath(resumeID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if len(text) > MaxHistorySize {
		text = text[len(text)-MaxHistorySize:]
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	if err := ensureInsideTranscriptsDir(path); err != nil {
		return err
	}
	if err := safeio.AtomicWriteFile(path, []byte(text), 0o600); err != nil {
		return fmt.Errorf("failed to write transcript history: %w", err)
	}
	return nil
}

// ReadHistory returns the history snapshot, the newest maxBytes of it when
// it is larger (maxBytes <= 0: everything). A transcript without a snapshot
// yields an empty Content and no error.
func ReadHistory(resumeID string, maxBytes int) (Content, error) {
	path, err := historyPath(resumeID)
	if err != nil {
		return Content{ResumeID: resumeID}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Content{ResumeID: resumeID}, nil
		}
		return Content{ResumeID: resumeID}, fmt.Errorf("failed to read transcript history: %w", err)
	}
	size := int64(len(data))
	if maxBytes > 0 && len(data) > maxBytes {
		data = data[len(data)-maxBytes:]
		// Start at a line, and never inside a UTF-8 sequence.
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			data = data[i+1:]
		}
		for len(data) > 0 && !utf8.RuneStart(data[0]) {
			data = data[1:]
		}
	}
	return Content{
		ResumeID:  resumeID,
		Text:      string(data),
		Size:      size,
		ReadBytes: int64(len(data)),
		Truncated: int64(len(data)) < size,
	}, nil
}

// deleteHistory removes the snapshot; a missing file is fine.
func deleteHistory(resumeID string) error {
	path, err := historyPath(resumeID)
	if err != nil {
		return err
	}
	if err := ensureInsideTranscriptsDir(path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete transcript history: %w", err)
	}
	return nil
}
