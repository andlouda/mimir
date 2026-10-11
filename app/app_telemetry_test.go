package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mimir/timeline"
)

func TestTelemetryEnvRendering(t *testing.T) {
	s := normalizeTelemetrySettings(TelemetrySettings{Enabled: true, OTelReceiver: true, Port: 14318, RequireToken: true, Token: "abc'def", ExportIntervalMs: 1000})
	env := telemetryEnv(s)
	joined := strings.Join(env, "\n")
	for _, want := range []string{"CLAUDE_CODE_ENABLE_TELEMETRY=1", "OTEL_LOGS_EXPORTER=otlp", "OTEL_METRICS_EXPORTER=none", "OTEL_EXPORTER_OTLP_PROTOCOL=http/json", "OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:14318", "OTEL_LOGS_EXPORT_INTERVAL=1000", "OTEL_EXPORTER_OTLP_HEADERS=x-mimir-token=abc'def"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "OTEL_LOG_USER_PROMPTS") || strings.Contains(joined, "TOOL_DETAILS") || strings.Contains(joined, "TRACES") {
		t.Fatalf("unexpected vars: %s", joined)
	}
	s.ToolDetails, s.TracesBeta, s.RequireToken = true, true, false
	joined = strings.Join(telemetryEnv(s), "\n")
	if !strings.Contains(joined, "OTEL_LOG_TOOL_DETAILS=1") || !strings.Contains(joined, "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1") || !strings.Contains(joined, "OTEL_TRACES_EXPORTER=otlp") || strings.Contains(joined, "x-mimir-token") {
		t.Fatalf("optional vars: %s", joined)
	}
	if telemetryEnv(TelemetrySettings{Enabled: false, OTelReceiver: true}) != nil {
		t.Fatal("no env while off")
	}
	a := &App{telemetry: normalizeTelemetrySettings(TelemetrySettings{Enabled: true, OTelReceiver: true, Port: 14318, RequireToken: true, Token: "abc'def"})}
	raw, err := a.GetTelemetryEnvBlockJSON()
	if err != nil {
		t.Fatal(err)
	}
	var block telemetryEnvBlock
	if err := json.Unmarshal([]byte(raw), &block); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(block.Posix, `export OTEL_EXPORTER_OTLP_HEADERS='x-mimir-token=abc'\''def'`) || !strings.Contains(block.PowerShell, `$env:OTEL_EXPORTER_OTLP_HEADERS = 'x-mimir-token=abc''def'`) || block.Endpoint != "http://127.0.0.1:14318" {
		t.Fatalf("block: %+v", block)
	}
	if !strings.Contains(block.Wsl, "/dev/tcp/127.0.0.1/14318") || !strings.Contains(block.Wsl, "x-mimir-token=abc") {
		t.Fatalf("wsl block: %s", block.Wsl)
	}
}

func TestTelemetryListenerIngestsIntoTimeline(t *testing.T) {
	a := &App{timeline: timeline.NewStore(filepath.Join(t.TempDir(), "tl"))}
	a.telemetry = normalizeTelemetrySettings(TelemetrySettings{Enabled: true, OTelReceiver: true, Port: 0, RequireToken: true, Token: "tok"})
	// Port 0: normalise clamps to 1024, so pick a free one through the server itself.
	a.telemetry.Port = 0
	a.startTelemetryListener()
	defer a.stopTelemetryListener()
	raw, _ := a.GetTelemetryStatusJSON()
	var st telemetryStatus
	_ = json.Unmarshal([]byte(raw), &st)
	if !st.Listening || !strings.HasPrefix(st.Addr, "127.0.0.1:") {
		t.Fatalf("status: %+v", st)
	}
	a.timeline.SetTerminal("sess-1", 4)
	body := `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"1760179321000000000","body":{"stringValue":"claude_code.api_request"},"attributes":[{"key":"session.id","value":{"stringValue":"sess-1"}},{"key":"model","value":{"stringValue":"claude-x"}},{"key":"user.email","value":{"stringValue":"x@y"}}]},{"body":{"stringValue":"other.event"},"attributes":[{"key":"session.id","value":{"stringValue":"sess-1"}}]}]}]}]}`
	req, _ := http.NewRequest(http.MethodPost, "http://"+st.Addr+"/v1/logs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mimir-Token", "tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("post: %d", resp.StatusCode)
	}
	deadline := time.Now().Add(2 * time.Second)
	var events []timeline.Event
	for time.Now().Before(deadline) {
		events, _ = a.timeline.Recent("sess-1", 0, 0)
		if len(events) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(events) != 1 || events[0].Type != "api_request" || events[0].TerminalID != 4 || events[0].Attrs["model"] != "claude-x" {
		t.Fatalf("stored: %+v", events)
	}
	if _, has := events[0].Attrs["user_email"]; has {
		t.Fatal("identity must be dropped")
	}
	raw, _ = a.GetTelemetryStatusJSON()
	_ = json.Unmarshal([]byte(raw), &st)
	if st.EventsReceived != 1 || st.DroppedForeign != 1 || st.LastEventAt == "" {
		t.Fatalf("counters: %+v", st)
	}
	// Wrong token is counted as rejected.
	req, _ = http.NewRequest(http.MethodPost, "http://"+st.Addr+"/v1/logs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	raw, _ = a.GetTelemetryStatusJSON()
	_ = json.Unmarshal([]byte(raw), &st)
	if resp.StatusCode != 401 || st.Rejected != 1 || !strings.HasPrefix(st.LastError, "401") {
		t.Fatalf("reject: %d %+v", resp.StatusCode, st)
	}
	a.stopTelemetryListener()
	raw, _ = a.GetTelemetryStatusJSON()
	var after telemetryStatus // fresh: omitted (empty) fields must not inherit old values
	_ = json.Unmarshal([]byte(raw), &after)
	if after.Listening || after.Addr != "" {
		t.Fatalf("after stop: %+v", after)
	}
}
