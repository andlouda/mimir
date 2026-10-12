package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"mimir/activitylog"
	"mimir/safeio"
)

// TelemetrySettings drive the agent timeline: which sources are collected
// and how long the records are kept. Everything is off until the user
// turns the master switch on; the sub-switches then default to on so one
// click gives the full picture.
type TelemetrySettings struct {
	// Enabled is the master switch ("Agent-Zeitleiste").
	Enabled bool `json:"enabled"`
	// HookToolEvents installs PreToolUse/PostToolUse/Stop hooks (tool ids,
	// timing, the agent's pid).
	HookToolEvents bool `json:"hookToolEvents"`
	// OTelReceiver listens for the agent's OpenTelemetry export (durations,
	// costs, permission decisions).
	OTelReceiver bool `json:"otelReceiver"`
	// Port of the loopback OTLP/HTTP listener.
	Port int `json:"port"`
	// RequireToken makes the listener demand the shared token header.
	RequireToken bool `json:"requireToken"`
	// WSLBridge (Windows) also listens on the WSL virtual adapter so Claude
	// Code inside WSL2 reaches the receiver; needs the token.
	WSLBridge bool `json:"wslBridge"`
	// Token is the shared secret; it stays in the file and is never sent to
	// the frontend (HasToken says whether one exists).
	Token    string `json:"token,omitempty"`
	HasToken bool   `json:"hasToken,omitempty"`
	// ToolDetails asks the agent to include tool parameters in telemetry
	// (OTEL_LOG_TOOL_DETAILS). Off: hooks already carry the redacted input.
	ToolDetails bool `json:"toolDetails"`
	// TracesBeta enables the agent's span export (subagent nesting).
	TracesBeta bool `json:"tracesBeta"`
	// ExportIntervalMs is how often the agent flushes telemetry.
	ExportIntervalMs int `json:"exportIntervalMs"`
	// Observe samples the pane's processes and connections.
	Observe           bool `json:"observe"`
	ObserveIntervalMs int  `json:"observeIntervalMs"`
	ObserveUDP        bool `json:"observeUdp"`
	// RetentionDays removes session files older than this (0 = keep).
	RetentionDays int `json:"retentionDays"`
	// MaxFileMB rotates a session file above this size.
	MaxFileMB int `json:"maxFileMb"`
}

const (
	telemetrySettingsFileName = "telemetry_settings.json"
	telemetryDefaultPort      = 14318
)

func defaultTelemetrySettings() TelemetrySettings {
	return TelemetrySettings{
		Enabled:           false,
		HookToolEvents:    true,
		OTelReceiver:      true,
		Port:              telemetryDefaultPort,
		RequireToken:      true,
		WSLBridge:         true,
		ToolDetails:       false,
		TracesBeta:        false,
		ExportIntervalMs:  1000,
		Observe:           true,
		ObserveIntervalMs: 1000,
		ObserveUDP:        false,
		RetentionDays:     14,
		MaxFileMB:         50,
	}
}

func clampInt(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func normalizeTelemetrySettings(s TelemetrySettings) TelemetrySettings {
	d := defaultTelemetrySettings()
	s.Port = clampInt(s.Port, 1024, 65535, d.Port)
	s.ExportIntervalMs = clampInt(s.ExportIntervalMs, 500, 60000, d.ExportIntervalMs)
	s.ObserveIntervalMs = clampInt(s.ObserveIntervalMs, 500, 10000, d.ObserveIntervalMs)
	if s.RetentionDays < 0 {
		s.RetentionDays = 0
	}
	if s.RetentionDays > 3650 {
		s.RetentionDays = 3650
	}
	s.MaxFileMB = clampInt(s.MaxFileMB, 1, 2048, d.MaxFileMB)
	if s.RequireToken && s.Token == "" {
		s.Token = newTelemetryToken()
	}
	s.HasToken = s.Token != ""
	return s
}

func newTelemetryToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func telemetrySettingsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}
	return filepath.Join(configDir, "mimir", telemetrySettingsFileName), nil
}

// timelineDir is where the per-session JSONL files live.
func timelineDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "mimir", "agent-timeline"), nil
}

// loadTelemetrySettings reads the file; a missing file means defaults, an
// unreadable one is logged and also means defaults (never crash start-up
// over a preference).
func loadTelemetrySettings() TelemetrySettings {
	path, err := telemetrySettingsPath()
	if err != nil {
		return normalizeTelemetrySettings(defaultTelemetrySettings())
	}
	s := defaultTelemetrySettings()
	if err := safeio.LoadJSON(path, &s); err != nil {
		// A broken file has been moved aside; the defaults are saved on
		// the next change and the old file stays for a look.
		log.Printf("telemetry settings unreadable, using defaults: %v", err)
		return normalizeTelemetrySettings(defaultTelemetrySettings())
	}
	return normalizeTelemetrySettings(s)
}

func saveTelemetrySettings(s TelemetrySettings) error {
	path, err := telemetrySettingsPath()
	if err != nil {
		return err
	}
	onDisk := s
	onDisk.HasToken = false
	payload, err := json.MarshalIndent(onDisk, "", "  ")
	if err != nil {
		return err
	}
	return safeio.AtomicWriteFile(path, payload, 0600)
}

// telemetrySettings is the current copy.
func (a *App) telemetrySettings() TelemetrySettings {
	a.telemetryMu.Lock()
	defer a.telemetryMu.Unlock()
	return a.telemetry
}

// hookToolEventsWanted says whether the installed hook set should include
// the tool events.
func (a *App) hookToolEventsWanted() bool {
	s := a.telemetrySettings()
	return s.Enabled && s.HookToolEvents
}

// telemetryView is what the frontend gets: no token value.
func telemetryView(s TelemetrySettings) TelemetrySettings {
	s.Token = ""
	return s
}

// GetTelemetrySettingsJSON returns the agent timeline settings.
func (a *App) GetTelemetrySettingsJSON() (string, error) {
	b, err := json.Marshal(telemetryView(a.telemetrySettings()))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UpdateTelemetrySettingsJSON applies a (partial) settings object: fields
// missing from the payload keep their value, the token can only be rotated
// through RotateTelemetryToken, never set from the frontend. Saves, applies
// the change (hook set) and logs it.
func (a *App) UpdateTelemetrySettingsJSON(raw string) (string, error) {
	old := a.telemetrySettings()
	next := old
	if err := json.Unmarshal([]byte(raw), &next); err != nil {
		return "", fmt.Errorf("invalid telemetry settings: %w", err)
	}
	next.Token = old.Token
	next = normalizeTelemetrySettings(next)
	if err := saveTelemetrySettings(next); err != nil {
		return "", fmt.Errorf("failed to save telemetry settings: %w", err)
	}
	a.telemetryMu.Lock()
	a.telemetry = next
	a.telemetryMu.Unlock()
	a.applyTelemetrySettings(old, next)
	b, err := json.Marshal(telemetryView(next))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// RotateTelemetryToken replaces the shared secret. Shells started before
// keep the old one in their environment and are refused from then on.
func (a *App) RotateTelemetryToken() error {
	old := a.telemetrySettings()
	next := old
	next.Token = newTelemetryToken()
	next = normalizeTelemetrySettings(next)
	if err := saveTelemetrySettings(next); err != nil {
		return err
	}
	a.telemetryMu.Lock()
	a.telemetry = next
	a.telemetryMu.Unlock()
	logTelemetryEvent("telemetry_token_rotated", "")
	a.applyTelemetrySettings(old, next)
	return nil
}

// applyTelemetrySettings reacts to a change: the hook set follows the
// master switch and the hook toggle; later PRs start or stop the listener
// and the observers here.
func (a *App) applyTelemetrySettings(old, next TelemetrySettings) {
	logTelemetryEvent("telemetry_settings_changed", fmt.Sprintf("enabled=%v hooks=%v otel=%v observe=%v port=%d retention=%dd", next.Enabled, next.HookToolEvents, next.OTelReceiver, next.Observe, next.Port, next.RetentionDays))
	oldHooks := old.Enabled && old.HookToolEvents
	newHooks := next.Enabled && next.HookToolEvents
	if oldHooks != newHooks {
		go a.upgradeOutdatedClaudeHooks()
	}
	if listenerKey(old) != listenerKey(next) {
		a.restartTelemetryListener()
	}
}

func logTelemetryEvent(event, reason string) {
	_ = activitylog.Append(activitylog.KindSecurityEvents, activitylog.SecurityEventEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     event,
		Operation: "agent_telemetry",
		Reason:    reason,
	})
}
