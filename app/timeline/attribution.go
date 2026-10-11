package timeline

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Attribution joins what the pane's process tree did with the tool call
// that caused it. A tool window opens at the hook's tool_pre and closes at
// its tool_post (or the telemetry's tool_result, whichever comes first).
// A process whose start falls into exactly one open window belongs to that
// call ("strong"); with several candidates the command line decides
// ("medium"), else the earliest window is guessed ("weak"); outside every
// window the process is unattributed ("none") and flagged. Connections
// inherit their process's attribution. Hook files reach Mimir a little
// late (the watcher polls), so runtime events are held for a moment before
// they are resolved.

const (
	// AttributionDelay is how long runtime events wait for the hook event
	// that may explain them (watcher interval plus a margin).
	AttributionDelay = 3 * time.Second
	// windowSlack widens a tool window on both sides: clocks and sampling
	// are not exact to the millisecond.
	windowSlack = time.Second
	// windowMaxAge drops windows nobody closed (hook lost, agent killed).
	windowMaxAge = time.Hour
	// maxHeld bounds the queue of unresolved runtime events.
	maxHeld = 2000
)

// Confidence levels.
const (
	ConfidenceStrong = "strong"
	ConfidenceMedium = "medium"
	ConfidenceWeak   = "weak"
	ConfidenceNone   = "none"
)

// Anomalies.
const (
	AnomalyUnattributedProcess    = "unattributed_process"
	AnomalyUnattributedConnection = "unattributed_connection"
	AnomalyOutlivedTool           = "outlived_tool"
	AnomalyToolFailed             = "tool_failed"
)

// Runtime event types.
const (
	TypeProcessStart = "process_start"
	TypeProcessEnd   = "process_end"
	TypeConnOpen     = "conn_open"
	TypeConnClose    = "conn_close"
)

type toolWindow struct {
	id      string
	tool    string
	command string
	start   time.Time
	end     time.Time
	closed  bool
}

type pidAttribution struct {
	toolUseID  string
	confidence string
}

// Attributor keeps the tool windows of one session and resolves runtime
// events against them.
type Attributor struct {
	mu      sync.Mutex
	windows map[string]*toolWindow
	held    []Event
	pids    map[int]pidAttribution
}

// NewAttributor creates an empty attributor.
func NewAttributor() *Attributor {
	return &Attributor{windows: map[string]*toolWindow{}, pids: map[int]pidAttribution{}}
}

// spawningTool reports whether a tool can start processes.
func spawningTool(name string) bool {
	switch name {
	case "Bash", "Task", "Agent", "Skill", "NotebookEdit":
		return true
	}
	return strings.HasPrefix(name, "mcp__")
}

// Observe feeds a hook or telemetry event: tool_pre opens a window,
// tool_post / tool_result close it.
func (a *Attributor) Observe(ev Event) {
	if ev.ToolUseID == "" {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, ev.TS)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case ev.Source == SourceHook && ev.Type == TypeToolPre:
		w := a.windows[ev.ToolUseID]
		if w == nil {
			w = &toolWindow{id: ev.ToolUseID}
			a.windows[ev.ToolUseID] = w
		}
		w.tool = ev.ToolName
		w.start = at
		if in, ok := ev.Attrs["tool_input"].(map[string]any); ok {
			if cmd, ok := in["command"].(string); ok {
				w.command = normaliseCommand(cmd)
			}
		}
	case (ev.Source == SourceHook && ev.Type == TypeToolPost) || (ev.Source == SourceOTel && ev.Type == "tool_result"):
		w := a.windows[ev.ToolUseID]
		if w == nil {
			// Post without pre (hook installed mid-call): a window from the
			// telemetry's duration, else a point.
			w = &toolWindow{id: ev.ToolUseID, tool: ev.ToolName, start: at}
			if d, ok := numberAttr(ev.Attrs["duration_ms"]); ok && d > 0 {
				w.start = at.Add(-time.Duration(d) * time.Millisecond)
			}
			a.windows[ev.ToolUseID] = w
		}
		if w.tool == "" {
			w.tool = ev.ToolName
		}
		if !w.closed || at.Before(w.end) {
			w.end = at
			w.closed = true
		}
	}
	a.pruneLocked(at)
}

// OpenWindows is the number of tool calls currently in flight (the
// observer samples faster while something runs).
func (a *Attributor) OpenWindows() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, w := range a.windows {
		if !w.closed {
			n++
		}
	}
	return n
}

// Hold queues a runtime event (proc / net) for later resolution.
func (a *Attributor) Hold(ev Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.held) >= maxHeld {
		a.held = a.held[len(a.held)-maxHeld+1:]
	}
	a.held = append(a.held, ev)
}

// Resolve returns the held events that are older than the delay, with
// confidence and anomaly filled in, in arrival order.
func (a *Attributor) Resolve(now time.Time) []Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []Event
	kept := a.held[:0]
	for _, ev := range a.held {
		at, err := time.Parse(time.RFC3339Nano, ev.TS)
		if err != nil {
			continue
		}
		if now.Sub(at) < AttributionDelay {
			kept = append(kept, ev)
			continue
		}
		out = append(out, a.resolveLocked(ev, at))
	}
	a.held = kept
	return out
}

func (a *Attributor) resolveLocked(ev Event, at time.Time) Event {
	switch ev.Type {
	case TypeProcessStart:
		started := at
		if s, ok := ev.Attrs["started_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				started = t
			}
		}
		cmd, _ := ev.Attrs["cmdline"].(string)
		id, conf := a.attributeLocked(started, cmd)
		if id == "" {
			ev.Confidence = ConfidenceNone
			ev.Anomaly = AnomalyUnattributedProcess
		} else {
			ev.ToolUseID = id
			ev.Confidence = conf
			if w := a.windows[id]; w != nil {
				ev.ToolName = w.tool
			}
		}
		if ev.PID > 0 {
			a.pids[ev.PID] = pidAttribution{toolUseID: id, confidence: ev.Confidence}
		}
	case TypeProcessEnd:
		if p, ok := a.pids[ev.PID]; ok {
			ev.ToolUseID = p.toolUseID
			ev.Confidence = p.confidence
			if w := a.windows[p.toolUseID]; w != nil {
				ev.ToolName = w.tool
				if w.closed && at.After(w.end.Add(windowSlack)) {
					ev.Anomaly = AnomalyOutlivedTool
				}
			}
			delete(a.pids, ev.PID)
		} else {
			ev.Confidence = ConfidenceNone
		}
	case TypeConnOpen, TypeConnClose:
		if p, ok := a.pids[ev.PID]; ok && p.toolUseID != "" {
			ev.ToolUseID = p.toolUseID
			ev.Confidence = p.confidence
			if w := a.windows[p.toolUseID]; w != nil {
				ev.ToolName = w.tool
			}
		} else {
			ev.Confidence = ConfidenceNone
			if ev.Type == TypeConnOpen {
				ev.Anomaly = AnomalyUnattributedConnection
			}
		}
	}
	return ev
}

// attributeLocked picks the window a process started in.
func (a *Attributor) attributeLocked(started time.Time, cmdline string) (string, string) {
	var candidates []*toolWindow
	for _, w := range a.windows {
		if !spawningTool(w.tool) || w.start.IsZero() {
			continue
		}
		if started.Before(w.start.Add(-windowSlack)) {
			continue
		}
		if w.closed && started.After(w.end.Add(windowSlack)) {
			continue
		}
		candidates = append(candidates, w)
	}
	switch len(candidates) {
	case 0:
		return "", ConfidenceNone
	case 1:
		return candidates[0].id, ConfidenceStrong
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].start.Before(candidates[j].start) })
	norm := normaliseCommand(cmdline)
	for _, w := range candidates {
		if w.command != "" && norm != "" && (strings.Contains(norm, w.command) || strings.Contains(w.command, norm)) {
			return w.id, ConfidenceMedium
		}
	}
	return candidates[0].id, ConfidenceWeak
}

// pruneLocked forgets windows that closed long ago and open ones nobody
// closed within the maximum age.
func (a *Attributor) pruneLocked(now time.Time) {
	for id, w := range a.windows {
		ref := w.end
		if !w.closed {
			ref = w.start
		}
		if !ref.IsZero() && now.Sub(ref) > windowMaxAge {
			delete(a.windows, id)
		}
	}
}

// normaliseCommand reduces a shell command for matching: whitespace
// collapsed, first 200 characters.
func normaliseCommand(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func numberAttr(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}
