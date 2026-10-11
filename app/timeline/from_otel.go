package timeline

import (
	"strings"
	"time"

	"mimir/otlp"
	"mimir/redact"
)

// OTelOptions tune the mapping of telemetry records.
type OTelOptions struct {
	// KeepToolDetails keeps tool_parameters / tool_input attributes (the
	// user turned OTEL_LOG_TOOL_DETAILS on). They are redacted either way.
	KeepToolDetails bool
}

// otelTypes maps Claude Code event names to timeline types.
var otelTypes = map[string]string{
	"claude_code.user_prompt":             "user_prompt",
	"claude_code.assistant_response":      "assistant_response",
	"claude_code.tool_result":             "tool_result",
	"claude_code.tool_decision":           "tool_decision",
	"claude_code.api_request":             "api_request",
	"claude_code.api_error":               "api_error",
	"claude_code.api_refusal":             "api_refusal",
	"claude_code.permission_mode_changed": "permission_mode_changed",
	"claude_code.mcp_server_connection":   "mcp_connection",
	"claude_code.skill_activated":         "skill_activated",
	"claude_code.auth":                    "auth",
	"claude_code.internal_error":          "internal_error",
}

// otelKeep lists the attributes stored per type. Anything not listed is
// dropped; identity (user.*, organization.id), prompt and response text
// never appear here.
var otelKeep = map[string][]string{
	"user_prompt":             {"prompt_length", "command_name", "command_source", "message.uuid"},
	"assistant_response":      {"response_length"},
	"tool_result":             {"success", "duration_ms", "decision_type", "decision_source", "error_type", "error", "tool_input_size_bytes", "tool_result_size_bytes", "mcp_server_scope"},
	"tool_decision":           {"decision", "source", "tool_source"},
	"api_request":             {"model", "cost_usd", "duration_ms", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "request_id", "speed", "query_source", "effort"},
	"api_error":               {"model", "error", "status_code", "duration_ms", "attempt", "request_id", "speed"},
	"api_refusal":             {"model", "request_id"},
	"permission_mode_changed": {"from_mode", "to_mode", "mode"},
	"mcp_connection":          {"server_name", "status", "error"},
	"skill_activated":         {"skill_name", "source"},
	"auth":                    {"status", "method"},
	"internal_error":          {"error_class", "component"},
}

var otelToolDetailKeys = []string{"tool_parameters", "tool_input"}

var otelCommon = []string{"terminal.type", "event.sequence"}

// FromOTelRecord maps one telemetry record to an event. Records without a
// valid session id or with a name outside claude_code.* are skipped (the
// boolean is false).
func FromOTelRecord(rec otlp.Record, opts OTelOptions) (Event, bool) {
	if !strings.HasPrefix(rec.Name, "claude_code.") {
		return Event{}, false
	}
	session, _ := rec.Attrs["session.id"].(string)
	if session == "" {
		session, _ = rec.Resource["session.id"].(string)
	}
	if !ValidSessionID(session) {
		return Event{}, false
	}
	at := rec.Time
	if at.IsZero() {
		at = rec.Observed
	}
	if at.IsZero() {
		at = time.Now()
	}
	ev := Event{
		V:         SchemaVersion,
		TS:        FormatTS(at),
		Source:    SourceOTel,
		SessionID: session,
		Attrs:     map[string]any{},
	}
	ev.Type = otelTypes[rec.Name]
	if ev.Type == "" {
		ev.Type = "otel_other"
		ev.Attrs["event"] = rec.Name
	}
	ev.PromptID, _ = rec.Attrs["prompt.id"].(string)
	ev.ToolUseID, _ = rec.Attrs["tool_use_id"].(string)
	ev.ToolName, _ = rec.Attrs["tool_name"].(string)
	keep := append(append([]string{}, otelKeep[ev.Type]...), otelCommon...)
	if opts.KeepToolDetails {
		keep = append(keep, otelToolDetailKeys...)
	}
	changed := false
	for _, k := range keep {
		v, ok := rec.Attrs[k]
		if !ok || v == nil {
			continue
		}
		rv, c := redact.Value(v, redact.Options{MaxString: 2048, KeepLong: []string{"full_command", "bash_command", "command"}, MaxLong: 4096, MaxItems: 32, MaxDepth: 4, Drop: []string{"prompt", "content", "old_string", "new_string"}})
		ev.Attrs[strings.ReplaceAll(k, ".", "_")] = rv
		changed = changed || c
	}
	ev.Redacted = changed
	if len(ev.Attrs) == 0 {
		ev.Attrs = nil
	}
	return ev, true
}

// spanTypes maps Claude Code span names to timeline types.
var spanTypes = map[string]string{
	"claude_code.interaction":          "span_interaction",
	"claude_code.llm_request":          "span_llm",
	"claude_code.tool":                 "span_tool",
	"claude_code.tool.execution":       "span_tool_exec",
	"claude_code.tool.blocked_on_user": "span_tool_blocked",
	"claude_code.hook":                 "span_hook",
}

var spanKeep = []string{"tool_name", "tool_use_id", "duration_ms", "result_tokens", "file_path", "model", "input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "ttft_ms", "success", "stop_reason", "request_id", "attempt", "error", "error_class", "interaction.sequence", "interaction.duration_ms", "user_prompt_length", "hook_event", "hook_name"}

// FromOTelSpan maps a span to an event. The event's time is the span start;
// duration_ms is derived from start and end when the span carries none.
func FromOTelSpan(sp otlp.Span, opts OTelOptions) (Event, bool) {
	typ, ok := spanTypes[sp.Name]
	if !ok {
		return Event{}, false
	}
	session, _ := sp.Attrs["session.id"].(string)
	if session == "" {
		session, _ = sp.Resource["session.id"].(string)
	}
	if !ValidSessionID(session) {
		return Event{}, false
	}
	at := sp.Start
	if at.IsZero() {
		at = time.Now()
	}
	ev := Event{
		V:         SchemaVersion,
		TS:        FormatTS(at),
		Source:    SourceOTel,
		Type:      typ,
		SessionID: session,
		Attrs:     map[string]any{"trace_id": sp.TraceID, "span_id": sp.SpanID},
	}
	if sp.ParentSpanID != "" {
		ev.Attrs["parent_span_id"] = sp.ParentSpanID
	}
	if !sp.Start.IsZero() && !sp.End.IsZero() && sp.End.After(sp.Start) {
		ev.Attrs["duration_ms"] = sp.End.Sub(sp.Start).Milliseconds()
	}
	if sp.StatusCode == 2 {
		ev.Attrs["status"] = "error"
		if sp.StatusMessage != "" {
			msg, _ := redact.String(sp.StatusMessage)
			ev.Attrs["status_message"] = msg
		}
	}
	ev.ToolUseID, _ = sp.Attrs["tool_use_id"].(string)
	ev.ToolName, _ = sp.Attrs["tool_name"].(string)
	keep := append([]string{}, spanKeep...)
	if opts.KeepToolDetails {
		keep = append(keep, "full_command", "user_prompt")
	}
	changed := false
	for _, k := range keep {
		v, ok := sp.Attrs[k]
		if !ok || v == nil {
			continue
		}
		if _, exists := ev.Attrs[strings.ReplaceAll(k, ".", "_")]; exists {
			continue
		}
		rv, c := redact.Value(v, redact.Options{MaxString: 2048, KeepLong: []string{"full_command"}, MaxLong: 4096, MaxItems: 32, MaxDepth: 4})
		ev.Attrs[strings.ReplaceAll(k, ".", "_")] = rv
		changed = changed || c
	}
	ev.Redacted = changed
	return ev, true
}
