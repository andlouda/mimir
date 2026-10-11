package timeline

import (
	"testing"
	"time"

	"mimir/otlp"
)

func TestFromOTelRecordWhitelistsAndRedacts(t *testing.T) {
	at := time.Unix(1760179321, 0)
	rec := otlp.Record{Time: at, Name: "claude_code.tool_result", Attrs: map[string]any{
		"session.id": "sess-1", "prompt.id": "p-1", "tool_use_id": "toolu_1", "tool_name": "Bash",
		"success": "true", "duration_ms": int64(1234), "decision_source": "config",
		"user.email": "me@example.com", "user.id": "u", "organization.id": "o", "terminal.type": "tmux", "event.sequence": int64(5),
		"tool_parameters": map[string]any{"bash_command": "curl -H 'Authorization: Bearer abcdefghijklmnop' https://x"},
	}}
	ev, ok := FromOTelRecord(rec, OTelOptions{})
	if !ok || ev.Type != "tool_result" || ev.Source != SourceOTel || ev.SessionID != "sess-1" || ev.PromptID != "p-1" || ev.ToolUseID != "toolu_1" || ev.ToolName != "Bash" {
		t.Fatalf("mapping: %+v", ev)
	}
	if ev.TS != FormatTS(at) {
		t.Fatalf("ts: %s", ev.TS)
	}
	if ev.Attrs["success"] != "true" || ev.Attrs["duration_ms"] != int64(1234) || ev.Attrs["terminal_type"] != "tmux" || ev.Attrs["event_sequence"] != int64(5) {
		t.Fatalf("kept attrs: %#v", ev.Attrs)
	}
	for _, k := range []string{"user.email", "user_email", "user.id", "organization.id", "tool_parameters", "prompt.id", "session.id"} {
		if _, bad := ev.Attrs[k]; bad {
			t.Fatalf("%s must not be stored: %#v", k, ev.Attrs)
		}
	}
	if ev.Redacted {
		t.Fatal("nothing was redacted")
	}
	// With tool details on, parameters are kept but secrets are removed.
	ev, _ = FromOTelRecord(rec, OTelOptions{KeepToolDetails: true})
	cmd := ev.Attrs["tool_parameters"].(map[string]any)["bash_command"].(string)
	if cmd != "curl -H 'Authorization: Bearer [REDACTED]' https://x" || !ev.Redacted {
		t.Fatalf("tool details: %q redacted=%v", cmd, ev.Redacted)
	}
	// Prompt text never survives, even for user_prompt.
	ev, ok = FromOTelRecord(otlp.Record{Observed: at, Name: "claude_code.user_prompt", Attrs: map[string]any{"session.id": "sess-1", "prompt": "secret plan", "prompt_length": int64(11)}}, OTelOptions{})
	if !ok || ev.Type != "user_prompt" || ev.Attrs["prompt"] != nil || ev.Attrs["prompt_length"] != int64(11) {
		t.Fatalf("user_prompt: %+v", ev)
	}
	// Unknown claude_code events keep their name; foreign records are dropped.
	ev, ok = FromOTelRecord(otlp.Record{Name: "claude_code.something_new", Attrs: map[string]any{"session.id": "sess-1"}}, OTelOptions{})
	if !ok || ev.Type != "otel_other" || ev.Attrs["event"] != "claude_code.something_new" {
		t.Fatalf("unknown: %+v", ev)
	}
	if _, ok := FromOTelRecord(otlp.Record{Name: "http.request", Attrs: map[string]any{"session.id": "sess-1"}}, OTelOptions{}); ok {
		t.Fatal("foreign record must be dropped")
	}
	if _, ok := FromOTelRecord(otlp.Record{Name: "claude_code.api_request", Attrs: map[string]any{"session.id": "../x"}}, OTelOptions{}); ok {
		t.Fatal("bad session id must be dropped")
	}
	if _, ok := FromOTelRecord(otlp.Record{Name: "claude_code.api_request"}, OTelOptions{}); ok {
		t.Fatal("missing session id must be dropped")
	}
}

func TestFromOTelSpan(t *testing.T) {
	start := time.Unix(1760179321, 0)
	sp := otlp.Span{TraceID: "t", SpanID: "2", ParentSpanID: "1", Name: "claude_code.tool", Start: start, End: start.Add(1500 * time.Millisecond),
		Attrs: map[string]any{"tool_name": "Bash", "tool_use_id": "toolu_1", "full_command": "ls"}, Resource: map[string]any{"session.id": "sess-1"}, StatusCode: 2, StatusMessage: "boom"}
	ev, ok := FromOTelSpan(sp, OTelOptions{})
	if !ok || ev.Type != "span_tool" || ev.ToolUseID != "toolu_1" || ev.Attrs["parent_span_id"] != "1" || ev.Attrs["duration_ms"] != int64(1500) || ev.Attrs["status"] != "error" || ev.Attrs["status_message"] != "boom" {
		t.Fatalf("span: %+v", ev)
	}
	if _, has := ev.Attrs["full_command"]; has {
		t.Fatal("full_command only with tool details")
	}
	ev, _ = FromOTelSpan(sp, OTelOptions{KeepToolDetails: true})
	if ev.Attrs["full_command"] != "ls" {
		t.Fatalf("details: %+v", ev.Attrs)
	}
	if _, ok := FromOTelSpan(otlp.Span{Name: "other"}, OTelOptions{}); ok {
		t.Fatal("unknown span must be dropped")
	}
}
