package main

import (
	"context"
	"log"
	"runtime"
	"time"

	"mimir/procwatch"
	"mimir/redact"
	"mimir/timeline"
)

// observeAgent samples the process tree below a pane's agent and writes
// what appears and disappears, with the sockets those processes hold, to
// the pane's timeline. It runs with the agent watcher's lifetime, only for
// local panes, only while the setting is on and a sampler exists for the
// platform, faster while a tool call is in flight and slower when the
// agent waits. The first snapshot is the baseline: processes that already
// ran when observation started are not reported as new.
func (a *App) observeAgent(ctx context.Context, terminalID int, state agentTerminalState) {
	if a.timeline == nil || state.source != "local" || state.pid <= 0 {
		return
	}
	s := a.telemetrySettings()
	if !s.Enabled || !s.Observe {
		return
	}
	sampler, err := procwatch.New()
	if err != nil {
		return
	}
	attributor := timeline.NewAttributor()
	unsub := a.timeline.Subscribe(func(ev timeline.Event) {
		if ev.TerminalID == terminalID && (ev.Source == timeline.SourceHook || ev.Source == timeline.SourceOTel) {
			attributor.Observe(ev)
		}
	})
	defer unsub()

	var prev *procwatch.Snapshot
	overflowNoted := false
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s := a.telemetrySettings()
		if !s.Enabled || !s.Observe {
			return
		}
		interval := time.Duration(s.ObserveIntervalMs) * time.Millisecond
		if runtime.GOOS == "windows" && interval < 2*time.Second {
			interval = 2 * time.Second // a Toolhelp snapshot plus per-process handles costs more than /proc
		}
		if attributor.OpenWindows() == 0 {
			interval *= 3
		}
		timer.Reset(interval)

		snap, err := sampler.Snapshot(state.pid, procwatch.Options{UDP: s.ObserveUDP})
		if err != nil {
			continue
		}
		now := snap.At
		session := a.timelineSessionFor(terminalID)
		if prev == nil {
			if session != "" {
				a.appendTimeline(timeline.Event{Source: timeline.SourceMimir, Type: "observe_started", SessionID: session, TerminalID: terminalID, TS: timeline.FormatTS(now), PID: state.pid, Attrs: map[string]any{"procs": len(snap.Procs), "conns": len(snap.Conns)}})
			}
			prev = &snap
			continue
		}
		if snap.Truncated && !overflowNoted && session != "" {
			overflowNoted = true
			a.appendTimeline(timeline.Event{Source: timeline.SourceMimir, Type: "overflow", SessionID: session, TerminalID: terminalID, TS: timeline.FormatTS(now), Attrs: map[string]any{"reason": "process tree or descriptor count above the observation cap"}})
		}
		for _, ch := range procwatch.Diff(prev, snap) {
			attributor.Hold(runtimeEvent(ch, terminalID))
		}
		prev = &snap
		for _, ev := range attributor.Resolve(now) {
			if ev.SessionID == "" {
				ev.SessionID = a.timelineSessionFor(terminalID)
			}
			if ev.SessionID == "" {
				continue // no session bound yet: nothing to file it under
			}
			a.appendTimeline(ev)
		}
	}
}

// runtimeEvent turns a snapshot change into a timeline event (session is
// filled in at resolution time). Command lines go through the redactor:
// a token on a curl command line is a secret like any other.
func runtimeEvent(ch procwatch.Change, terminalID int) timeline.Event {
	ev := timeline.Event{
		V:          timeline.SchemaVersion,
		TS:         timeline.FormatTS(ch.At),
		TSKind:     timeline.TSKindObserved,
		TerminalID: terminalID,
		Attrs:      map[string]any{},
	}
	switch ch.Kind {
	case procwatch.ProcessStart, procwatch.ProcessEnd:
		ev.Source = timeline.SourceProc
		ev.Type = ch.Kind
		ev.PID = ch.Proc.PID
		ev.PPID = ch.Proc.PPID
		cmd, changed := redact.String(ch.Proc.Cmdline)
		if len(cmd) > 4096 {
			cmd = cmd[:4096] + "...[truncated]"
			changed = true
		}
		if cmd != "" {
			ev.Attrs["cmdline"] = cmd
		}
		ev.Redacted = changed
		if !ch.Proc.StartedAt.IsZero() {
			ev.Attrs["started_at"] = timeline.FormatTS(ch.Proc.StartedAt)
			if ch.Kind == procwatch.ProcessEnd {
				ev.Attrs["lifetime_ms"] = ch.At.Sub(ch.Proc.StartedAt).Milliseconds()
			}
		}
	case procwatch.ConnOpen, procwatch.ConnClose:
		ev.Source = timeline.SourceNet
		ev.Type = ch.Kind
		ev.PID = ch.Conn.PID
		ev.Attrs["proto"] = ch.Conn.Proto
		ev.Attrs["local"] = ch.Conn.Local
		ev.Attrs["remote"] = ch.Conn.Remote
		if ch.Conn.State != "" {
			ev.Attrs["state"] = ch.Conn.State
		}
		ev.Attrs["inode"] = ch.Conn.Inode
	}
	return ev
}

func (a *App) appendTimeline(ev timeline.Event) {
	if _, err := a.timeline.Append(ev); err != nil {
		log.Printf("agent timeline: %v", err)
	}
}
