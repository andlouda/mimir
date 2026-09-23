package agents

import (
	"encoding/json"
	"strings"
)

// State is what an agent is doing right now, derived from the tail of its
// session file rather than from window titles or screen contents.
type State string

const (
	StateUnknown State = "unknown"
	StateWorking State = "working"
	// StateIdle means the agent finished its turn and waits for the user.
	StateIdle State = "idle"
	// StatePermission means the agent is blocked on a permission prompt
	// (known only through the notification hook).
	StatePermission State = "permission"
)

// StateInfo is the derived state plus the text the user most likely wants
// to see next to it (the agent's latest answer, first lines only).
type StateInfo struct {
	State State `json:"state"`
	// LastText is the newest assistant text (trimmed to a few hundred
	// characters) — the question or summary the agent is waiting on.
	LastText string `json:"lastText,omitempty"`
	// LastAt is the timestamp of the record the state was derived from.
	LastAt string `json:"lastAt,omitempty"`
}

const lastTextMax = 400

// DeriveState inspects the tail of a session file for kind.
func DeriveState(kind Kind, tail []byte) StateInfo {
	switch kind {
	case KindClaude:
		return deriveClaudeState(tail)
	case KindCodex:
		return deriveCodexState(tail)
	default:
		return StateInfo{State: StateUnknown}
	}
}

// deriveClaudeState: Claude Code appends a record per API message. While the
// model works there is nothing new yet, so the last record is the user's
// prompt or a tool result; a tool_use record means a tool is running. Only
// a final assistant text record — with no tool call after it — means the
// turn is over and the agent waits for input.
func deriveClaudeState(tail []byte) StateInfo {
	info := StateInfo{State: StateUnknown}
	last := ""
	lastText := ""
	lastTextAt := ""
	scanLines(tail, func(line []byte) {
		var rec claudeRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return
		}
		switch rec.Type {
		case "user":
			if rec.IsMeta {
				return
			}
			last = "user"
			info.LastAt = rec.Timestamp
		case "assistant":
			blocks := claudeBlocks(rec.Message.Content)
			hasTool := false
			for _, b := range blocks {
				if b.Type == "tool_use" {
					hasTool = true
				}
			}
			if text := claudeTextContent(rec.Message.Content, false); text != "" {
				lastText, lastTextAt = text, rec.Timestamp
			}
			if hasTool {
				last = "tool"
			} else if len(blocks) > 0 || len(rec.Message.Content) > 0 {
				last = "answer"
			}
			info.LastAt = rec.Timestamp
		}
	})
	switch last {
	case "user", "tool":
		info.State = StateWorking
	case "answer":
		info.State = StateIdle
	}
	if info.State == StateIdle || info.State == StateWorking {
		info.LastText = trimText(lastText)
		if info.State == StateIdle && lastTextAt != "" {
			info.LastAt = lastTextAt
		}
	}
	return info
}

// deriveCodexState: Codex writes explicit task_started / task_complete
// (or turn_aborted) events around every turn.
func deriveCodexState(tail []byte) StateInfo {
	info := StateInfo{State: StateUnknown}
	lastText := ""
	scanLines(tail, func(line []byte) {
		var rec codexRecord
		if err := json.Unmarshal(line, &rec); err != nil || rec.Type != "event_msg" {
			return
		}
		switch rec.Payload.Type {
		case "task_started":
			info.State, info.LastAt = StateWorking, rec.Timestamp
		case "task_complete", "turn_aborted":
			info.State, info.LastAt = StateIdle, rec.Timestamp
		case "agent_message":
			if t := strings.TrimSpace(rec.Payload.Message); t != "" {
				lastText = t
			}
		}
	})
	if info.State != StateUnknown {
		info.LastText = trimText(lastText)
	}
	return info
}

func trimText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > lastTextMax {
		cut := text[:lastTextMax]
		if i := strings.LastIndex(cut, " "); i > lastTextMax/2 {
			cut = cut[:i]
		}
		return cut + "…"
	}
	return text
}
