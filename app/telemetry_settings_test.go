package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTelemetrySettingsDefaultsAndNormalise(t *testing.T) {
	d := normalizeTelemetrySettings(defaultTelemetrySettings())
	if d.Enabled || !d.HookToolEvents || !d.OTelReceiver || d.Port != telemetryDefaultPort || d.ToolDetails || d.RetentionDays != 14 {
		t.Fatalf("defaults: %+v", d)
	}
	if !d.RequireToken || len(d.Token) != 64 || !d.HasToken {
		t.Fatalf("a token must be generated when required: %+v", d)
	}
	n := normalizeTelemetrySettings(TelemetrySettings{Port: 80, ExportIntervalMs: 10, ObserveIntervalMs: 99999, RetentionDays: -1, MaxFileMB: 0})
	if n.Port != 1024 || n.ExportIntervalMs != 500 || n.ObserveIntervalMs != 10000 || n.RetentionDays != 0 || n.MaxFileMB != 50 {
		t.Fatalf("clamping: %+v", n)
	}
	if view := telemetryView(d); view.Token != "" || !view.HasToken {
		t.Fatalf("the view must hide the token: %+v", view)
	}
}

func TestUpdateTelemetrySettingsIsPartialAndKeepsToken(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("HOME", cfg) // macOS: UserConfigDir is $HOME/Library/Application Support
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("APPDATA", cfg)
	a := &App{telemetry: loadTelemetrySettings()}
	token := a.telemetry.Token
	out, err := a.UpdateTelemetrySettingsJSON(`{"enabled":true,"retentionDays":3,"token":"attacker"}`)
	if err != nil {
		t.Fatal(err)
	}
	var view TelemetrySettings
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Enabled || view.RetentionDays != 3 || !view.HookToolEvents || view.Token != "" {
		t.Fatalf("update result: %+v", view)
	}
	if s := a.telemetrySettings(); s.Token != token || !s.Enabled || !s.HookToolEvents {
		t.Fatal("token must be kept and the hook set wanted")
	}
	path, err := telemetrySettingsPath()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"token": "`+token+`"`) || strings.Contains(string(raw), "hasToken") {
		t.Fatalf("file content: %s", raw)
	}
	if _, err := a.UpdateTelemetrySettingsJSON(`{bad`); err == nil {
		t.Fatal("invalid json must be refused")
	}
	reloaded := loadTelemetrySettings()
	if !reloaded.Enabled || reloaded.Token != token {
		t.Fatalf("reload: %+v", reloaded)
	}
}
