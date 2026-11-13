package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func maskForAgent(t *testing.T, app *App, pane int, text string) agentMaskResult {
	t.Helper()
	raw, err := app.MaskTextForAgentJSON(pane, text)
	if err != nil {
		t.Fatalf("MaskTextForAgentJSON: %v", err)
	}
	var out agentMaskResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func unmaskFromAgent(t *testing.T, app *App, pane int, text string) agentUnmaskResult {
	t.Helper()
	raw, err := app.UnmaskTextFromAgentJSON(pane, text)
	if err != nil {
		t.Fatalf("UnmaskTextFromAgentJSON: %v", err)
	}
	var out agentUnmaskResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestAgentBridgeKeepsOneMappingPerAgentPaneAcrossMessages(t *testing.T) {
	freshConfigDir(t)
	app := &App{}

	first := maskForAgent(t, app, 7, "db 10.0.3.17 refuses, web 10.0.3.18 ok")
	if first.Text != "db MIMIR_IP_1 refuses, web MIMIR_IP_2 ok" || first.Masked["IP"] != 2 {
		t.Fatalf("first = %#v", first)
	}
	// A later message: the known host keeps its placeholder, the count is
	// for this message only.
	second := maskForAgent(t, app, 7, "still 10.0.3.17, new 10.0.3.19")
	if second.Text != "still MIMIR_IP_1, new MIMIR_IP_3" || second.Masked["IP"] != 2 {
		t.Fatalf("second = %#v", second)
	}

	back := unmaskFromAgent(t, app, 7, "ssh root@MIMIR_IP_1 'pg_isready -h MIMIR_IP_3'")
	if back.Text != "ssh root@10.0.3.17 'pg_isready -h 10.0.3.19'" || back.Restored != 2 || len(back.Unknown) != 0 {
		t.Fatalf("back = %#v", back)
	}
}

func TestAgentBridgeScrubsSecretsIrreversibly(t *testing.T) {
	freshConfigDir(t)
	app := &App{}
	out := maskForAgent(t, app, 7, "export API_TOKEN=abc123verysecret\nhost 10.0.3.17")
	if strings.Contains(out.Text, "abc123verysecret") || !strings.Contains(out.Text, "[REDACTED]") {
		t.Fatalf("secret not scrubbed: %q", out.Text)
	}
}

func TestAgentBridgeMappingsAreSeparatePerPaneAndDroppedOnClose(t *testing.T) {
	freshConfigDir(t)
	app := &App{}
	maskForAgent(t, app, 7, "host 10.0.3.17")

	// Another agent pane never learned that placeholder.
	other := unmaskFromAgent(t, app, 8, "ping MIMIR_IP_1")
	if other.Text != "ping MIMIR_IP_1" || len(other.Unknown) != 1 {
		t.Fatalf("other pane resolved a foreign placeholder: %#v", other)
	}

	app.agentMasks.forget(7)
	gone := unmaskFromAgent(t, app, 7, "ping MIMIR_IP_1")
	if gone.Text != "ping MIMIR_IP_1" || len(gone.Unknown) != 1 {
		t.Fatalf("mapping survived the pane: %#v", gone)
	}
}
