package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"

	"mimir/otlp"
	"mimir/terminal"
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
	// WslAddr is the second listen address on the WSL adapter (Windows).
	WslAddr string `json:"wslAddr,omitempty"`
}

// telemetryListenerWanted says whether the receiver should run.
func telemetryListenerWanted(s TelemetrySettings) bool {
	return s.Enabled && s.OTelReceiver
}

// listenerKey captures the settings that need a restart when they change.
func listenerKey(s TelemetrySettings) string {
	return fmt.Sprintf("%v|%d|%v|%s|%v|%v", telemetryListenerWanted(s), s.Port, s.RequireToken, s.Token, s.TracesBeta, s.WSLBridge)
}

// wslAdapter finds the Windows host's address on the WSL virtual switch
// ("vEthernet (WSL...)"): the address WSL2 distros see as their default
// gateway. Empty when there is none (no WSL, mirrored networking, other
// platforms).
func wslAdapter() (net.IP, *net.IPNet) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil
	}
	for _, ifc := range ifaces {
		if !strings.Contains(strings.ToLower(ifc.Name), "wsl") || ifc.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				return n.IP.To4(), n
			}
		}
	}
	return nil, nil
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
	a.telemetryStat.WslAddr = ""
	a.telemetryMu.Unlock()
	logTelemetryEvent("telemetry_listener_started", srv.ListenAddr())
	if s.WSLBridge && token != "" {
		if ip, ipnet := wslAdapter(); ip != nil {
			wsl := &otlp.Server{Addr: net.JoinHostPort(ip.String(), strconv.Itoa(s.Port)), Token: token, MaxBody: otlp.DefaultMaxBody, AcceptTraces: s.TracesBeta, AllowFrom: []*net.IPNet{ipnet}, OnLogs: srv.OnLogs, OnSpans: srv.OnSpans, OnReject: srv.OnReject}
			if err := wsl.Start(); err != nil {
				log.Printf("telemetry WSL listener: %v", err)
				logTelemetryEvent("telemetry_bind_failed", "wsl: "+err.Error())
			} else {
				a.telemetryMu.Lock()
				a.telemetryWSLServer = wsl
				a.telemetryStat.WslAddr = wsl.ListenAddr()
				a.telemetryMu.Unlock()
				logTelemetryEvent("telemetry_listener_started", "wsl: "+wsl.ListenAddr())
			}
		}
	}
}

// stopTelemetryListener shuts the receiver down if it runs.
func (a *App) stopTelemetryListener() {
	a.telemetryMu.Lock()
	srv := a.telemetryServer
	wsl := a.telemetryWSLServer
	a.telemetryServer, a.telemetryWSLServer = nil, nil
	a.telemetryStat.Listening = false
	addr := a.telemetryStat.Addr
	a.telemetryStat.Addr = ""
	a.telemetryStat.WslAddr = ""
	a.telemetryMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if wsl != nil {
		_ = wsl.Stop(ctx)
	}
	if srv == nil {
		return
	}
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

// telemetryEnvBlock is the copyable form of telemetryEnv. Wsl is the POSIX
// form for a WSL2 distro: it probes 127.0.0.1 first (mirrored networking)
// and falls back to the default gateway, which is the Windows host.
type telemetryEnvBlock struct {
	Posix      string `json:"posix"`
	PowerShell string `json:"powershell"`
	Wsl        string `json:"wsl"`
	Endpoint   string `json:"endpoint"`
}

// GetTelemetryEnvBlockJSON renders the export lines for a shell, for users
// who start Claude Code outside Mimir's own panes. It includes the token
// (the user needs it to configure their shell); it is never sent otherwise.
func (a *App) GetTelemetryEnvBlockJSON() (string, error) {
	s := a.telemetrySettings()
	env := telemetryEnv(s)
	var ps strings.Builder
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		ps.WriteString("$env:" + k + " = '" + strings.ReplaceAll(v, "'", "''") + "'\n")
	}
	block := telemetryEnvBlock{Posix: terminal.RenderExports(env, false), PowerShell: ps.String(), Wsl: terminal.RenderExports(env, true), Endpoint: "http://127.0.0.1:" + strconv.Itoa(s.Port)}
	b, err := json.Marshal(block)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
