// Package timeline is the per-session record of what an agent did and what
// happened around it: hook events (tool calls with their ids and timing),
// telemetry the agent exports, and the processes and connections observed
// in its pane. Events are normalised into one schema so sources can be
// joined by session id, tool_use_id and pid rather than by timestamps
// alone, and written as JSONL per session so other tools can read them.
package timeline

import (
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
	"time"

	"mimir/agents"
)

// SchemaVersion is written into every event as "v".
const SchemaVersion = 1

// Sources of events.
const (
	SourceHook       = "hook"
	SourceOTel       = "otel"
	SourceTranscript = "transcript"
	SourceProc       = "proc"
	SourceNet        = "net"
	SourceMimir      = "mimir"
)

// Timestamp kinds: "" means the source's own clock.
const (
	TSKindFileMtime = "file_mtime"
	TSKindObserved  = "observed"
)

// Event is one line of <session_id>.jsonl.
type Event struct {
	V      int    `json:"v"`
	Seq    uint64 `json:"seq"`
	TS     string `json:"ts"`
	TSKind string `json:"ts_kind,omitempty"`
	Source string `json:"source"`
	Type   string `json:"type"`

	SessionID  string `json:"session_id"`
	TerminalID int    `json:"terminal_id,omitempty"`
	PromptID   string `json:"prompt_id,omitempty"`
	ToolUseID  string `json:"tool_use_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	PID        int    `json:"pid,omitempty"`
	PPID       int    `json:"ppid,omitempty"`

	Attrs      map[string]any `json:"attrs,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
	Redacted   bool           `json:"redacted,omitempty"`
	Anomaly    string         `json:"anomaly,omitempty"`
}

// sessionIDPattern bounds what may become a file name: ids come from hook
// payloads and, later, from HTTP bodies.
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// ErrInvalidSession rejects ids that are not safe as file names.
var ErrInvalidSession = errors.New("invalid session id")

// ValidSessionID reports whether id may be used as a store key.
func ValidSessionID(id string) bool {
	return sessionIDPattern.MatchString(id) && id != "." && id != ".."
}

// Validate checks the fields a store relies on.
func (e Event) Validate() error {
	if !ValidSessionID(e.SessionID) {
		return ErrInvalidSession
	}
	if e.Source == "" || e.Type == "" {
		return errors.New("event without source or type")
	}
	return nil
}

// FormatTS renders a time the way every event carries it.
func FormatTS(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// SessionIDFromTranscript derives the session id from a transcript path
// (Claude Code names the file after the session).
func SessionIDFromTranscript(file string) string {
	base := path.Base(strings.ReplaceAll(file, `\`, "/"))
	return strings.TrimSuffix(base, ".jsonl")
}

// HookSessionID is the session an event belongs to: the payload's id, else
// the transcript's base name.
func HookSessionID(ev agents.HookEvent) string {
	if ev.SessionID != "" {
		return ev.SessionID
	}
	return SessionIDFromTranscript(ev.TranscriptPath)
}

// Hook event types.
const (
	TypeSessionStart = "session_start"
	TypePromptSubmit = "prompt_submit"
	TypeToolPre      = "tool_pre"
	TypeToolPost     = "tool_post"
	TypeStop         = "stop"
	TypeSubagentStop = "subagent_stop"
	TypeNotification = "notification"
)

// NewFromHook maps a hook payload to an event. at is the time the hook
// wrote its file (the payload itself carries no clock); terminalID is the
// pane the watcher matched, 0 when unknown. The hook process already
// compacted and redacted the payload, so the attrs are stored as they are.
func NewFromHook(ev agents.HookEvent, at time.Time, terminalID int) (Event, bool) {
	out := Event{
		V:          SchemaVersion,
		TS:         FormatTS(at),
		TSKind:     TSKindFileMtime,
		Source:     SourceHook,
		SessionID:  HookSessionID(ev),
		TerminalID: terminalID,
		PID:        ev.PPID,
		Attrs:      map[string]any{},
	}
	if ev.TranscriptPath != "" {
		out.Attrs["transcript_path"] = ev.TranscriptPath
		if agents.IsSubagentTranscript(ev.TranscriptPath) {
			out.Attrs["subagent"] = true
		}
	}
	if ev.AgentID != "" {
		out.Attrs["agent_id"] = ev.AgentID
	}
	switch ev.HookEventName {
	case "SessionStart":
		out.Type = TypeSessionStart
		if ev.Cwd != "" {
			out.Attrs["cwd"] = ev.Cwd
		}
		if ev.StartSource != "" {
			out.Attrs["start_source"] = ev.StartSource
		}
	case "UserPromptSubmit":
		out.Type = TypePromptSubmit
	case "PreToolUse", "PostToolUse":
		out.Type = TypeToolPre
		if ev.HookEventName == "PostToolUse" {
			out.Type = TypeToolPost
			if len(ev.ToolResponse) > 0 {
				var resp any
				if json.Unmarshal(ev.ToolResponse, &resp) == nil {
					out.Attrs["response"] = resp
				}
			}
		}
		out.ToolName = ev.ToolName
		out.ToolUseID = ev.ToolUseID
		if len(ev.ToolInput) > 0 {
			var in any
			if json.Unmarshal(ev.ToolInput, &in) == nil {
				out.Attrs["tool_input"] = in
				out.Redacted = strings.Contains(string(ev.ToolInput), "[REDACTED]") || strings.Contains(string(ev.ToolInput), "...[truncated]")
			}
		}
		if ev.Cwd != "" && ev.HookEventName == "PreToolUse" {
			out.Attrs["cwd"] = ev.Cwd
		}
	case "Stop":
		out.Type = TypeStop
		if ev.StopHookActive {
			out.Attrs["stop_hook_active"] = true
		}
	case "SubagentStop":
		out.Type = TypeSubagentStop
	case "Notification":
		out.Type = TypeNotification
		out.Attrs["notification_type"] = ev.NotificationType
		if ev.Message != "" {
			out.Attrs["message"] = ev.Message
		}
	default:
		return Event{}, false
	}
	if len(out.Attrs) == 0 {
		out.Attrs = nil
	}
	if out.Validate() != nil {
		return Event{}, false
	}
	return out, true
}
