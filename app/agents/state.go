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
	// Activity names the tool call still running ("Bash: npm test",
	// "Edit: main.go") while the state is working; empty while the model
	// itself is thinking.
	Activity string `json:"activity,omitempty"`
	// ActivityAt is when that tool call started.
	ActivityAt string `json:"activityAt,omitempty"`
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
	// Tool calls without a result yet, in call order (parallel calls are
	// answered one by one; the newest still-open one is shown).
	type openTool struct{ id, summary, at string }
	var open []openTool
	scanLines(tail, func(line []byte) {
		var rec claudeRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return
		}
		switch rec.Type {
		case "user":
			for _, b := range claudeBlocks(rec.Message.Content) {
				if b.Type != "tool_result" || b.ToolUseID == "" {
					continue
				}
				for i := range open {
					if open[i].id == b.ToolUseID {
						open = append(open[:i], open[i+1:]...)
						break
					}
				}
			}
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
					open = append(open, openTool{id: b.ID, summary: claudeToolSummary(b.Name, b.Input), at: rec.Timestamp})
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
	if info.State == StateWorking && len(open) > 0 {
		newest := open[len(open)-1]
		info.Activity, info.ActivityAt = newest.summary, newest.at
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
	type openCall struct{ id, summary, at string }
	var open []openCall
	scanLines(tail, func(line []byte) {
		var rec codexRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return
		}
		if rec.Type == "response_item" {
			p := rec.Payload
			switch p.Type {
			case "function_call":
				if p.Name != "exec_command" && p.Name != "shell" {
					return
				}
				var args codexExecArgs
				_ = json.Unmarshal([]byte(p.Arguments), &args)
				cmd := strings.TrimSpace(args.Cmd)
				if cmd == "" && len(args.Command) > 0 {
					cmd = strings.Join(args.Command, " ")
				}
				if r := []rune(cmd); len(r) > toolSummaryMax {
					cmd = string(r[:toolSummaryMax]) + "…"
				}
				open = append(open, openCall{id: p.CallID, summary: "shell: " + cmd, at: rec.Timestamp})
			case "function_call_output":
				for i := range open {
					if open[i].id == p.CallID {
						open = append(open[:i], open[i+1:]...)
						break
					}
				}
			}
			return
		}
		if rec.Type != "event_msg" {
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
	if info.State == StateWorking && len(open) > 0 {
		newest := open[len(open)-1]
		info.Activity, info.ActivityAt = newest.summary, newest.at
	}
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
