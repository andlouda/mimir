package agents

import (
	"encoding/json"
	"errors"
	"fmt"
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

// HookEvent is the payload Claude Code writes to a Notification hook.
type HookEvent struct {
	SessionID        string `json:"session_id"`
	TranscriptPath   string `json:"transcript_path"`
	Cwd              string `json:"cwd"`
	HookEventName    string `json:"hook_event_name"`
	NotificationType string `json:"notification_type"`
	Message          string `json:"message"`
	AgentID          string `json:"agent_id"`
	// PPID is the process that ran the hook command, i.e. the agent itself.
	// It is not part of the payload; the hook command encodes it in the file
	// name, which makes the pane assignment exact even when several panes
	// share a working directory or a session file.
	PPID int `json:"-"`
}

// ParseHookEvent decodes a payload and rejects anything that is not a
// notification (the directory is only ever written by the hook, but a stray
// file must not crash the watcher).
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
	case "SessionStart":
		// Binds the pane to its session file the moment Claude starts
		// (or resumes / clears / compacts); carries no state.
		if ev.TranscriptPath == "" && ev.SessionID == "" {
			return ev, errors.New("session start without a session")
		}
	default:
		return ev, errors.New("unsupported hook event")
	}
	return ev, nil
}

// hookEvents lists the Claude Code events Mimir subscribes to, with the
// matcher for each ("" = every occurrence).
var hookEvents = []struct{ Event, Matcher string }{
	{"Notification", HookMatcher},
	{"SessionStart", ""},
}

// StateForNotification maps a notification type to an agent state.
// Permission prompts and elicitations block the agent on the user; idle
// prompts and agent_needs_input mean it is waiting for input.
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

// InstallClaudeHook returns settings.json content with Mimir's Notification
// hook added (or refreshed). Other settings are preserved; an empty or
// missing file yields a minimal document.
func InstallClaudeHook(settings []byte, command string, args []string) ([]byte, error) {
	doc, err := parseSettings(settings)
	if err != nil {
		return nil, err
	}
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for _, he := range hookEvents {
		var entries []hookEntry
		if raw, ok := hooks[he.Event]; ok {
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
		kept = append(kept, hookEntry{Matcher: he.Matcher, Hooks: []map[string]any{mimirHook(command, args)}})
		hooks[he.Event] = kept
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
	for _, he := range hookEvents {
		raw, ok := hooks[he.Event]
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
			delete(hooks, he.Event)
		} else {
			hooks[he.Event] = kept
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

// HookUpToDate reports whether every event Mimir subscribes to has its
// entry; an older install (Notification only) is reported as outdated so
// the UI can offer a refresh.
func HookUpToDate(settings []byte) bool {
	for _, he := range hookEvents {
		if !hasMimirHookFor(settings, he.Event) {
			return false
		}
	}
	return true
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

// ScanHookEvents reads all payloads in dir, oldest first. Unparseable or
// stale files are removed when fs can delete; a missing directory is not an
// error.
func ScanHookEvents(fs FS, dir string) []HookEventFile {
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return nil
	}
	remover, _ := fs.(Remover)
	var out []HookEventFile
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(e.Name, ".json") {
			continue
		}
		p := fs.Join(dir, e.Name)
		stale := time.Since(e.ModTime) > hookEventMaxAge
		data, err := fs.ReadHead(p, 64*1024)
		if err != nil {
			continue
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
