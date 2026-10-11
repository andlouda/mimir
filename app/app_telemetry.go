package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"mimir/otlp"
	"mimir/timeline"
)

// telemetryStatus is what the settings card shows about the receiver.
type telemetryStatus struct {
	Listening      bool   `json:"listening"`
	Addr           string `json:"addr,omitempty"`
	EventsReceived int64  `json:"eventsReceived"`
	SpansReceived  int64  `json:"spansReceived"`
	DroppedUnbound int64  `json:"droppedUnbound"`
	DroppedForeign int64  `json:"droppedForeign"`
	Rejected       int64  `json:"rejected"`
	LastEventAt    string `json:"lastEventAt,omitempty"`
	LastError      string `json:"lastError,omitempty"`
}

// telemetryListenerWanted says whether the receiver should run.
func telemetryListenerWanted(s TelemetrySettings) bool {
	return s.Enabled && s.OTelReceiver
}

// listenerKey captures the settings that need a restart when they change.
func listenerKey(s TelemetrySettings) string {
	return fmt.Sprintf("%v|%d|%v|%s|%v", telemetryListenerWanted(s), s.Port, s.RequireToken, s.Token, s.TracesBeta)
}

// startTelemetryListener binds the OTLP receiver according to the current
// settings. Errors land in the status (and the security log), never in a
// crash: a port in use must not take the terminal down.
func (a *App) startTelemetryListener() {
	s := a.telemetrySettings()
	if !telemetryListenerWanted(s) || a.timeline == nil {
		return
	}
	token := ""
	if s.RequireToken {
		token = s.Token
	}
	srv := &otlp.Server{
		Addr:         "127.0.0.1:" + strconv.Itoa(s.Port),
		Token:        token,
		MaxBody:      otlp.DefaultMaxBody,
		AcceptTraces: s.TracesBeta,
		OnLogs:       a.ingestOTelRecords,
		OnSpans:      a.ingestOTelSpans,
		OnReject: func(status int, reason string) {
			a.telemetryMu.Lock()
			a.telemetryStat.Rejected++
			a.telemetryStat.LastError = fmt.Sprintf("%d %s", status, reason)
			a.telemetryMu.Unlock()
		},
	}
	a.telemetryMu.Lock()
	if a.telemetryServer != nil {
		a.telemetryMu.Unlock()
		return
	}
	a.telemetryMu.Unlock()
	if err := srv.Start(); err != nil {
		a.telemetryMu.Lock()
		a.telemetryStat.Listening = false
		a.telemetryStat.Addr = ""
		a.telemetryStat.LastError = err.Error()
		a.telemetryMu.Unlock()
		log.Printf("telemetry listener: %v", err)
		logTelemetryEvent("telemetry_bind_failed", err.Error())
		return
	}
	a.telemetryMu.Lock()
	a.telemetryServer = srv
	a.telemetryStat.Listening = true
	a.telemetryStat.Addr = srv.ListenAddr()
	a.telemetryStat.LastError = ""
	a.telemetryMu.Unlock()
	logTelemetryEvent("telemetry_listener_started", srv.ListenAddr())
}

// stopTelemetryListener shuts the receiver down if it runs.
func (a *App) stopTelemetryListener() {
	a.telemetryMu.Lock()
	srv := a.telemetryServer
	a.telemetryServer = nil
	a.telemetryStat.Listening = false
	addr := a.telemetryStat.Addr
	a.telemetryStat.Addr = ""
	a.telemetryMu.Unlock()
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Stop(ctx)
	logTelemetryEvent("telemetry_listener_stopped", addr)
}

// restartTelemetryListener applies changed listener settings.
func (a *App) restartTelemetryListener() {
	a.stopTelemetryListener()
	a.startTelemetryListener()
}

func (a *App) ingestOTelRecords(recs []otlp.Record) {
	if a.timeline == nil {
		return
	}
	opts := timeline.OTelOptions{KeepToolDetails: a.telemetrySettings().ToolDetails}
	var received, foreign, unbound int64
	for _, rec := range recs {
		ev, ok := timeline.FromOTelRecord(rec, opts)
		if !ok {
			foreign++
			continue
		}
		if _, err := a.timeline.Append(ev); err != nil {
			if err == timeline.ErrTooManySessions {
				unbound++
			}
			continue
		}
		received++
	}
	a.noteTelemetry(received, 0, foreign, unbound)
}

func (a *App) ingestOTelSpans(spans []otlp.Span) {
	if a.timeline == nil {
		return
	}
	opts := timeline.OTelOptions{KeepToolDetails: a.telemetrySettings().ToolDetails}
	var received, foreign, unbound int64
	for _, sp := range spans {
		ev, ok := timeline.FromOTelSpan(sp, opts)
		if !ok {
			foreign++
			continue
		}
		if _, err := a.timeline.Append(ev); err != nil {
			if err == timeline.ErrTooManySessions {
				unbound++
			}
			continue
		}
		received++
	}
	a.noteTelemetry(0, received, foreign, unbound)
}

func (a *App) noteTelemetry(events, spans, foreign, unbound int64) {
	a.telemetryMu.Lock()
	defer a.telemetryMu.Unlock()
	a.telemetryStat.EventsReceived += events
	a.telemetryStat.SpansReceived += spans
	a.telemetryStat.DroppedForeign += foreign
	a.telemetryStat.DroppedUnbound += unbound
	if events+spans > 0 {
		a.telemetryStat.LastEventAt = time.Now().Format(time.RFC3339)
	}
}

// GetTelemetryStatusJSON reports the receiver's state and counters.
func (a *App) GetTelemetryStatusJSON() (string, error) {
	a.telemetryMu.Lock()
	st := a.telemetryStat
	a.telemetryMu.Unlock()
	b, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// telemetryEnv lists the KEY=VALUE pairs that make Claude Code export to
// Mimir's receiver. Prompt text is never requested
// (OTEL_LOG_USER_PROMPTS stays unset); metrics are turned off.
func telemetryEnv(s TelemetrySettings) []string {
	if !telemetryListenerWanted(s) {
		return nil
	}
	interval := strconv.Itoa(s.ExportIntervalMs)
	env := []string{
		"CLAUDE_CODE_ENABLE_TELEMETRY=1",
		"OTEL_LOGS_EXPORTER=otlp",
		"OTEL_METRICS_EXPORTER=none",
		"OTEL_EXPORTER_OTLP_PROTOCOL=http/json",
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:" + strconv.Itoa(s.Port),
		"OTEL_LOGS_EXPORT_INTERVAL=" + interval,
	}
	if s.RequireToken && s.Token != "" {
		env = append(env, "OTEL_EXPORTER_OTLP_HEADERS=x-mimir-token="+s.Token)
	}
	if s.ToolDetails {
		env = append(env, "OTEL_LOG_TOOL_DETAILS=1")
	}
	if s.TracesBeta {
		env = append(env, "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1", "OTEL_TRACES_EXPORTER=otlp", "OTEL_TRACES_EXPORT_INTERVAL="+interval)
	}
	return env
}

// telemetryEnvBlock is the copyable form of telemetryEnv.
type telemetryEnvBlock struct {
	Posix      string `json:"posix"`
	PowerShell string `json:"powershell"`
	Endpoint   string `json:"endpoint"`
}

// GetTelemetryEnvBlockJSON renders the export lines for a shell, for users
// who start Claude Code outside Mimir's own panes. It includes the token
// (the user needs it to configure their shell); it is never sent otherwise.
func (a *App) GetTelemetryEnvBlockJSON() (string, error) {
	s := a.telemetrySettings()
	env := telemetryEnv(s)
	var posix, ps strings.Builder
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		posix.WriteString("export " + k + "=" + posixQuoteValue(v) + "\n")
		ps.WriteString("$env:" + k + " = '" + strings.ReplaceAll(v, "'", "''") + "'\n")
	}
	block := telemetryEnvBlock{Posix: posix.String(), PowerShell: ps.String(), Endpoint: "http://127.0.0.1:" + strconv.Itoa(s.Port)}
	b, err := json.Marshal(block)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// posixQuoteValue single-quotes a value unless it is plainly safe.
func posixQuoteValue(v string) string {
	safe := v != ""
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=", r)) {
			safe = false
			break
		}
	}
	if safe {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}
