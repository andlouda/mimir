package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"mimir/redact"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Claude Code notification hook. Claude Code asks for permission before
// running tools, and Mimir cannot tell "tool running" from "waiting for
// approval" by looking at the session file. Claude Code's Notification hook
// is the supported channel for that: on every permission prompt (and a few
// related events) it runs a command with a JSON payload on stdin. Mimir's
// hook command writes that payload into a small events directory, which the
// session watcher reads. Nothing is parsed off the screen, and installing
// the hook is explicit and reversible.

// HookMarker identifies Mimir's hook entry inside settings.json.
const HookMarker = "mimir-agent-hook"

// HookEventsDirName is the directory (below the platform cache dir) where
// hook payloads are dropped.
const HookEventsDirName = "agent-events"

// HookMatcher lists the notification types Mimir reacts to.
const HookMatcher = "permission_prompt|idle_prompt|agent_needs_input|elicitation_dialog"

// HookEvent is the payload Claude Code writes to one of Mimir's hooks:
// notifications and session events always, tool events (PreToolUse,
// PostToolUse, Stop, SubagentStop) when the agent timeline is on.
type HookEvent struct {
	SessionID        string `json:"session_id"`
	TranscriptPath   string `json:"transcript_path"`
	Cwd              string `json:"cwd"`
	HookEventName    string `json:"hook_event_name"`
	NotificationType string `json:"notification_type"`
	Message          string `json:"message"`
	AgentID          string `json:"agent_id"`
	// StartSource is SessionStart's "source": startup, resume, clear, compact.
	StartSource string `json:"source"`
	// Tool events. ToolInput is the compacted, redacted argument object
	// (see CompactHookEvent); ToolResponse is reduced to a summary
	// {bytes, is_error, interrupted} because the output itself is already
	// in the transcript and may be large.
	ToolName       string          `json:"tool_name"`
	ToolUseID      string          `json:"tool_use_id"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`
	StopHookActive bool            `json:"stop_hook_active"`
	// PPID is the process that ran the hook command, i.e. the agent itself.
	// It is not part of the payload; the hook command encodes it in the file
	// name, which makes the pane assignment exact even when several panes
	// share a working directory or a session file.
	PPID int `json:"-"`
}

// ParseHookEvent decodes a payload and rejects anything that is not one of
// the events Mimir subscribes to (the directory is only ever written by the
// hook, but a stray file must not crash the watcher).
func ParseHookEvent(data []byte) (HookEvent, error) {
	var ev HookEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return ev, err
	}
	switch ev.HookEventName {
	case "Notification":
		if ev.NotificationType == "" {
			return ev, errors.New("notification without a type")
		}
	case "PreToolUse", "PostToolUse":
		if ev.ToolName == "" || ev.SessionID == "" {
			return ev, errors.New("tool event without tool name or session")
		}
	case "Stop", "SubagentStop":
		if ev.TranscriptPath == "" && ev.SessionID == "" {
			return ev, errors.New("stop event without a session")
		}
	case "SessionStart", "UserPromptSubmit":
		// Bind the pane to its session file: the moment Claude starts (or
		// resumes / clears / compacts) and on every prompt, so sessions
		// that were already running when the hook was installed get bound
		// at their next prompt. Carries no state; the prompt text in a
		// UserPromptSubmit payload is never read.
		if ev.TranscriptPath == "" && ev.SessionID == "" {
			return ev, errors.New("session event without a session")
		}
	default:
		return ev, errors.New("unsupported hook event")
	}
	return ev, nil
}

// hookEventSpec is one Claude Code hook event with its matcher ("" = every
// occurrence).
type hookEventSpec struct{ Event, Matcher string }

// baseHookEvents are always installed: they drive prompts and the pane
// binding and carry no tool data.
var baseHookEvents = []hookEventSpec{
	{"Notification", HookMatcher},
	{"SessionStart", ""},
	{"UserPromptSubmit", ""},
}

// toolHookEvents are installed only when the agent timeline is on: they
// name every tool call with its id and timing.
var toolHookEvents = []hookEventSpec{
	{"PreToolUse", ""},
	{"PostToolUse", ""},
	{"Stop", ""},
	{"SubagentStop", ""},
}

// HookEventsFor lists the events Mimir wants installed.
func HookEventsFor(toolEvents bool) []hookEventSpec {
	if !toolEvents {
		return baseHookEvents
	}
	return append(append([]hookEventSpec{}, baseHookEvents...), toolHookEvents...)
}

// IsToolHookEvent reports whether an event name belongs to the tool set.
func IsToolHookEvent(name string) bool {
	for _, he := range toolHookEvents {
		if he.Event == name {
			return true
		}
	}
	return false
}

// hookEventNames is every event Mimir has ever installed; removal sweeps
// them all.
func hookEventNames() []string {
	out := make([]string, 0, len(baseHookEvents)+len(toolHookEvents))
	for _, he := range HookEventsFor(true) {
		out = append(out, he.Event)
	}
	return out
}

// Caps for compacted hook payloads: the argument object of a tool call is
// kept whole up to this size, strings inside it to maxHookString bytes,
// except the shell command, which is the one value worth keeping in full.
const (
	MaxHookInputBytes = 4 * 1024
	maxHookString     = 512
	maxHookItems      = 32
	maxHookDepth      = 4
)

// hookDropKeys are removed from every payload: the prompt text (never read
// by Mimir), file contents and edit strings (they are in the transcript).
var hookDropKeys = []string{"prompt", "content", "old_string", "new_string", "edits", "todos"}

// CompactHookEvent reduces a raw hook payload to what the timeline keeps:
// the tool response becomes {bytes, is_error, interrupted}, the tool input
// is redacted and capped, prompt text and file contents are dropped. The
// result is what the hook process writes, so secrets never reach disk.
func CompactHookEvent(data []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if resp, ok := doc["tool_response"]; ok {
		doc["tool_response"] = summarizeToolResponse(resp)
	}
	if in, ok := doc["tool_input"]; ok {
		v, _ := redact.Value(in, redact.Options{MaxString: maxHookString, KeepLong: []string{"command"}, MaxLong: MaxHookInputBytes, MaxItems: maxHookItems, MaxDepth: maxHookDepth, Drop: hookDropKeys})
		doc["tool_input"] = v
	}
	for _, k := range hookDropKeys {
		delete(doc, k)
	}
	// Anything else (message, cwd, paths) still goes through the string
	// redaction.
	out, _ := redact.Value(doc, redact.Options{MaxString: MaxHookInputBytes, MaxItems: maxHookItems, MaxDepth: maxHookDepth + 2})
	return json.Marshal(out)
}

// summarizeToolResponse keeps only the size and the error flags of a tool
// result. Bash results carry "interrupted"; generic results an "is_error"
// or an "error" string.
func summarizeToolResponse(resp any) map[string]any {
	raw, _ := json.Marshal(resp)
	out := map[string]any{"bytes": len(raw)}
	if m, ok := resp.(map[string]any); ok {
		if b, ok := m["interrupted"].(bool); ok && b {
			out["interrupted"] = true
		}
		if b, ok := m["is_error"].(bool); ok && b {
			out["is_error"] = true
		}
		if e, ok := m["error"].(string); ok && e != "" {
			out["is_error"] = true
		}
	}
	return out
}

// StateForNotification maps a notification type to an agent state.
// Permission prompts and elicitations block the agent on the user; idle
// prompts and agent_needs_input mean it is waiting for input.
// IsSubagentTranscript reports whether a transcript path belongs to a
// subagent (Claude Code keeps them under <session>/subagents/). Hook
// events for a subagent's prompt carry that path; the pane must keep
// following the parent session, and the prompt is cleared when the
// subagent's own file moves on.
func IsSubagentTranscript(path string) bool {
	p := strings.ReplaceAll(path, "\\", "/")
	return strings.Contains(p, "/subagents/")
}

func StateForNotification(notificationType string) State {
	switch notificationType {
	case "permission_prompt", "elicitation_dialog":
		return StatePermission
	case "idle_prompt", "agent_needs_input":
		return StateIdle
	default:
		return StateUnknown
	}
}

// LocalHookCommand is the exec-form hook that runs Mimir itself; no shell is
// involved, so it works identically on Linux, macOS and Windows.
func LocalHookCommand(executable string) (command string, args []string) {
	return executable, []string{"--agent-hook"}
}

// RemoteHookCommand is a POSIX one-liner for hosts without a Mimir binary
// (Claude Code over SSH). It drops the payload below ~/.cache/mimir.
func RemoteHookCommand() string {
	dir := `"$HOME/.cache/mimir/` + HookEventsDirName + `"`
	return `mkdir -p ` + dir + ` && umask 077 && cat > ` + dir + `/"$(date +%s)-$$-$PPID.json"`
}

// HookEventFileName names an event file: <stamp>-<pid>-<ppid>.json. The
// remote one-liner produces the same shape with $$ and $PPID.
func HookEventFileName(stamp int64, pid, ppid int) string {
	return strconv.FormatInt(stamp, 10) + "-" + strconv.Itoa(pid) + "-" + strconv.Itoa(ppid) + ".json"
}

// hookEventPPID extracts the third numeric field of an event file name; 0
// when absent (older hook installs wrote <stamp>-<pid>.json).
func hookEventPPID(name string) int {
	name = strings.TrimSuffix(name, ".json")
	parts := strings.Split(name, "-")
	if len(parts) < 3 {
		return 0
	}
	n, err := strconv.Atoi(parts[2])
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// hookEntry is one element of hooks.Notification.
type hookEntry struct {
	Matcher string           `json:"matcher,omitempty"`
	Hooks   []map[string]any `json:"hooks"`
}

// mimirHook is the settings.json entry. The hook command must never print
// to stdout and never exit non-zero: for PreToolUse that would feed a
// permission decision back into Claude Code, and Mimir only observes.
func mimirHook(command string, args []string) map[string]any {
	h := map[string]any{
		"type":          "command",
		"command":       command,
		"statusMessage": HookMarker,
		"timeout":       10,
	}
	if len(args) > 0 {
		h["args"] = args
	}
	return h
}

func isMimirHook(h map[string]any) bool {
	if s, ok := h["statusMessage"].(string); ok && s == HookMarker {
		return true
	}
	if c, ok := h["command"].(string); ok && strings.Contains(c, HookEventsDirName) {
		return true
	}
	if args, ok := h["args"].([]any); ok {
		for _, a := range args {
			if s, ok := a.(string); ok && s == "--agent-hook" {
				return true
			}
		}
	}
	return false
}

// InstallClaudeHook returns settings.json content with Mimir's hooks added
// (or refreshed) for the wanted event set and removed from the others, so
// turning tool events off also takes their entries out. Other settings and
// foreign hooks are preserved; an empty or missing file yields a minimal
// document.
func InstallClaudeHook(settings []byte, command string, args []string, toolEvents bool) ([]byte, error) {
	doc, err := parseSettings(settings)
	if err != nil {
		return nil, err
	}
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	wanted := map[string]hookEventSpec{}
	for _, he := range HookEventsFor(toolEvents) {
		wanted[he.Event] = he
	}
	for _, event := range hookEventNames() {
		var entries []hookEntry
		if raw, ok := hooks[event]; ok {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &entries)
		}
		kept := entries[:0]
		for _, e := range entries {
			var rest []map[string]any
			for _, h := range e.Hooks {
				if !isMimirHook(h) {
					rest = append(rest, h)
				}
			}
			if len(rest) > 0 {
				e.Hooks = rest
				kept = append(kept, e)
			}
		}
		if he, ok := wanted[event]; ok {
			kept = append(kept, hookEntry{Matcher: he.Matcher, Hooks: []map[string]any{mimirHook(command, args)}})
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	doc["hooks"] = hooks
	return json.MarshalIndent(doc, "", "  ")
}

// RemoveClaudeHook returns settings.json content without Mimir's hook.
func RemoveClaudeHook(settings []byte) ([]byte, bool, error) {
	doc, err := parseSettings(settings)
	if err != nil {
		return nil, false, err
	}
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		return settings, false, nil
	}
	removed := false
	for _, event := range hookEventNames() {
		raw, ok := hooks[event]
		if !ok {
			continue
		}
		var entries []hookEntry
		b, _ := json.Marshal(raw)
		_ = json.Unmarshal(b, &entries)
		var kept []hookEntry
		changed := false
		for _, e := range entries {
			var rest []map[string]any
			for _, h := range e.Hooks {
				if isMimirHook(h) {
					changed = true
					continue
				}
				rest = append(rest, h)
			}
			if len(rest) > 0 {
				e.Hooks = rest
				kept = append(kept, e)
			}
		}
		if !changed {
			continue
		}
		removed = true
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	if !removed {
		return settings, false, nil
	}
	if len(hooks) == 0 {
		delete(doc, "hooks")
	} else {
		doc["hooks"] = hooks
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	return out, true, err
}

// HasClaudeHook reports whether Mimir's hook is present in settings.json
// (the Notification entry, which every install has had).
func HasClaudeHook(settings []byte) bool {
	return hasMimirHookFor(settings, "Notification")
}

// HookUpToDate reports whether the installed set is exactly the wanted
// one: every wanted event has its entry and no unwanted one does. An older
// install (Notification only) is outdated, and so is a tool-event install
// after the user turned the timeline off; the UI offers a refresh and the
// start-up upgrade applies it.
func HookUpToDate(settings []byte, toolEvents bool) bool {
	wanted := map[string]bool{}
	for _, he := range HookEventsFor(toolEvents) {
		wanted[he.Event] = true
	}
	for _, event := range hookEventNames() {
		if hasMimirHookFor(settings, event) != wanted[event] {
			return false
		}
	}
	return true
}

// HasToolHooks reports whether the tool-event entries are installed.
func HasToolHooks(settings []byte) bool {
	return hasMimirHookFor(settings, "PreToolUse")
}

func hasMimirHookFor(settings []byte, event string) bool {
	doc, err := parseSettings(settings)
	if err != nil {
		return false
	}
	hooks, _ := doc["hooks"].(map[string]any)
	raw, ok := hooks[event]
	if !ok {
		return false
	}
	var entries []hookEntry
	b, _ := json.Marshal(raw)
	_ = json.Unmarshal(b, &entries)
	for _, e := range entries {
		for _, h := range e.Hooks {
			if isMimirHook(h) {
				return true
			}
		}
	}
	return false
}

func parseSettings(settings []byte) (map[string]any, error) {
	doc := map[string]any{}
	if strings.TrimSpace(string(settings)) == "" {
		return doc, nil
	}
	if err := json.Unmarshal(settings, &doc); err != nil {
		return nil, fmt.Errorf("settings.json is not valid JSON: %w", err)
	}
	return doc, nil
}

// Remover is implemented by filesystems that can delete processed hook
// events. FS itself stays read-only for the transcript lookup.
type Remover interface {
	Remove(file string) error
}

// HookEventFile is one payload found in the events directory.
type HookEventFile struct {
	Path    string
	ModTime time.Time
	Event   HookEvent
}

// hookEventMaxAge: events nobody consumed (Claude Code outside Mimir, a
// closed pane) are garbage-collected after this.
const hookEventMaxAge = 10 * time.Minute

// hookEventHardMaxAge is measured with the local clock and only clears
// events nobody will ever match (pane closed long ago).
const hookEventHardMaxAge = 24 * time.Hour

// maxHookFileBytes bounds one event file. Local files are compacted by the
// hook process and stay small; a remote PostToolUse with a huge response
// beyond this is dropped (its result is in the transcript anyway).
const maxHookFileBytes = 256 * 1024

// ScanHookEvents reads all payloads in dir, oldest first. Unparseable or
// stale files are removed when fs can delete; a missing directory is not an
// error.
func ScanHookEvents(fs FS, dir string) []HookEventFile {
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return nil
	}
	remover, _ := fs.(Remover)
	// Mtimes of SSH/WSL hosts come from their clocks; judge age against the
	// newest event in the directory so a skewed remote clock does not
	// throw every event away before it is matched. A local hard cap
	// still clears leftovers.
	var newest time.Time
	for _, e := range entries {
		if !e.IsDir && strings.HasSuffix(e.Name, ".json") && e.ModTime.After(newest) {
			newest = e.ModTime
		}
	}
	var out []HookEventFile
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(e.Name, ".json") {
			continue
		}
		p := fs.Join(dir, e.Name)
		stale := newest.Sub(e.ModTime) > hookEventMaxAge || time.Since(e.ModTime) > hookEventHardMaxAge
		data, err := fs.ReadHead(p, maxHookFileBytes)
		if err != nil {
			continue
		}
		// Files written by the remote one-liner are raw; compact them here
		// so stored events are the same shape wherever they came from.
		if compact, cerr := CompactHookEvent(data); cerr == nil {
			data = compact
		}
		ev, perr := ParseHookEvent(data)
		if perr != nil || stale {
			if remover != nil {
				_ = remover.Remove(p)
			}
			continue
		}
		ev.PPID = hookEventPPID(e.Name)
		out = append(out, HookEventFile{Path: p, ModTime: e.ModTime, Event: ev})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.Before(out[j].ModTime) })
	return out
}

// MatchesHookEvent reports whether an event belongs to a pane. The agent's
// pid decides when both sides know it (the hook runs as the agent's child);
// otherwise the session file (by session id in the file name, so path styles
// do not matter) and, when no file is known yet, the working directory.
func MatchesHookEvent(ev HookEvent, agentPID int, sessionFile, cwd string) bool {
	if ev.PPID > 0 && agentPID > 0 {
		return ev.PPID == agentPID
	}
	if sessionFile != "" {
		base := path.Base(strings.ReplaceAll(sessionFile, `\`, "/"))
		if ev.SessionID != "" && base == ev.SessionID+".jsonl" {
			return true
		}
		if ev.TranscriptPath != "" && path.Base(strings.ReplaceAll(ev.TranscriptPath, `\`, "/")) == base {
			return true
		}
		return false
	}
	return cwd != "" && ev.Cwd != "" && strings.TrimRight(ev.Cwd, "/\\") == strings.TrimRight(cwd, "/\\")
}

// HookEventStub is what an event file name alone tells: the hook's parent
// pid (the agent) and when it was written. Listing stubs opens no file, so
// it is cheap enough to run for panes nobody has matched yet.
type HookEventStub struct {
	Name    string
	PPID    int
	ModTime time.Time
}

// ListHookEventStubs lists the event files of dir by name only, without
// reading or removing anything. A missing directory yields nothing.
func ListHookEventStubs(fs FS, dir string) []HookEventStub {
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []HookEventStub
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(e.Name, ".json") {
			continue
		}
		out = append(out, HookEventStub{Name: e.Name, PPID: hookEventPPID(e.Name), ModTime: e.ModTime})
	}
	return out
}
