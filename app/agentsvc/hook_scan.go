package agentsvc

import (
	"context"
	"fmt"
	"runtime"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"mimir/agents"
)

// The hook mailbox is normally read by the watcher of a pane whose agent
// was already detected. Detection itself is event-driven (window titles,
// start-up banners), and some starts produce neither: `claude -r` in a
// tmux pane prints no banner, and tmux keeps the title inside. The agent's
// own hook still fires (SessionStart, the first prompt), so an event whose
// parent pid belongs to no detected pane is the signal that some pane
// needs a probe. The scanner lists the mailbox by file name only (no
// reads, no deletes) and asks the frontend to re-probe the panes of that
// host; the pane's watcher then consumes the file as usual.
const (
	agentHookScanInterval = 3 * time.Second
	// agentHookHintCooldown bounds the probes a busy foreign agent (Claude
	// outside Mimir) can trigger.
	agentHookHintCooldown = 5 * time.Second
	// agentHookHintPerPID: one hint per agent pid per interval; a probe
	// that found nothing for it will not find it on the next event either.
	agentHookHintPerPID = 10 * time.Minute
	// agentHookScanWSLEvery: the WSL mailbox is reached over \\wsl$, so it
	// is listed on every n-th tick only.
	agentHookScanWSLEvery = 2
	agentHookWSLRetry     = 5 * time.Minute
)

// hookScanSource is one mailbox (local cache dir, or the default WSL
// distro's) with what the scanner remembers about it.
type hookScanSource struct {
	name     string // local | wsl
	fs       agents.FS
	dir      string
	seen     map[string]struct{}
	hinted   map[int]time.Time
	primed   bool
	lastHint time.Time
}

func newHookScanSource(name string, fs agents.FS, dir string) *hookScanSource {
	return &hookScanSource{name: name, fs: fs, dir: dir, seen: map[string]struct{}{}, hinted: map[int]time.Time{}}
}

// unboundPPIDs returns the parent pids of event files that appeared since
// the previous call and belong to none of the known agent pids, at most
// once per pid per agentHookHintPerPID. The first call only records what
// is there: leftovers from before Mimir started are not a reason to probe.
func (s *hookScanSource) unboundPPIDs(stubs []agents.HookEventStub, known map[int]bool, now time.Time) []int {
	current := make(map[string]struct{}, len(stubs))
	var out []int
	for _, st := range stubs {
		current[st.Name] = struct{}{}
		if _, ok := s.seen[st.Name]; ok {
			continue
		}
		if !s.primed || st.PPID <= 0 || known[st.PPID] {
			continue
		}
		if last, ok := s.hinted[st.PPID]; ok && now.Sub(last) < agentHookHintPerPID {
			continue
		}
		s.hinted[st.PPID] = now
		out = append(out, st.PPID)
	}
	s.seen = current
	s.primed = true
	for pid, at := range s.hinted {
		if now.Sub(at) > agentHookHintPerPID {
			delete(s.hinted, pid)
		}
	}
	return out
}

// scanUnboundHookEvents runs until ctx ends. It does nothing while
// detection is off or every open pane already has an agent.
func (a *Service) scanUnboundHookEvents(ctx context.Context) {
	ticker := time.NewTicker(agentHookScanInterval)
	defer ticker.Stop()
	var sources []*hookScanSource
	if dir, err := localHookEventsDir(); err == nil {
		sources = append(sources, newHookScanSource("local", agents.LocalFS{}, dir))
	}
	var wsl *hookScanSource
	var wslTried time.Time
	tick := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		tick++
		if !a.IsAgentDetectionEnabled() || !a.hasAgentlessTerminal() {
			continue
		}
		if runtime.GOOS == "windows" && wsl == nil && time.Since(wslTried) > agentHookWSLRetry {
			wslTried = time.Now()
			if base, home, err := wslHomeUNC(); err == nil {
				fs := agents.LocalFS{Base: base}
				wsl = newHookScanSource("wsl", fs, hookEventsDir("wsl", fs, home))
				sources = append(sources, wsl)
			}
		}
		known := a.knownAgentPIDs()
		now := time.Now()
		for _, src := range sources {
			if src == wsl && tick%agentHookScanWSLEvery != 0 {
				continue
			}
			ppids := src.unboundPPIDs(agents.ListHookEventStubs(src.fs, src.dir), known[src.name], now)
			if len(ppids) == 0 || now.Sub(src.lastHint) < agentHookHintCooldown {
				continue
			}
			src.lastHint = now
			logAgentEvent("agent_detection_hint", fmt.Sprintf("%s: hook event from agent pid %d that no pane claims", src.name, ppids[0]))
			wailsruntime.EventsEmit(a.ctx, "agent-detect-hint", map[string]any{"source": src.name, "pid": ppids[0]})
		}
	}
}

// knownAgentPIDs is the detected agent pid per source ("local", "wsl",
// "ssh"); pid spaces differ per host, so they are kept apart.
func (a *Service) knownAgentPIDs() map[string]map[int]bool {
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	out := map[string]map[int]bool{}
	for _, st := range a.agentStates {
		if st.pid <= 0 {
			continue
		}
		if out[st.source] == nil {
			out[st.source] = map[int]bool{}
		}
		out[st.source][st.pid] = true
	}
	return out
}

// hasAgentlessTerminal reports whether a pane without a detected agent is
// open (SSH panes aside: their mailbox is not reachable without a session).
func (a *Service) hasAgentlessTerminal() bool {
	ids := a.terminals.SessionIDs()
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	for _, id := range ids {
		if _, ok := a.agentStates[id]; ok {
			continue
		}
		if a.terminals.GetSSHClient(id) != nil {
			continue
		}
		return true
	}
	return false
}

// noteDetectionOutcome records why a probe found nothing, once per
// distinct reason per pane, so a pane that stays without a badge leaves a
// trace in the security events instead of silence.
func (a *Service) noteDetectionOutcome(terminalID int, terminalType string, res agentDetectionResult) {
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	if a.agentDetectReasons == nil {
		a.agentDetectReasons = map[int]string{}
	}
	if res.Detected {
		delete(a.agentDetectReasons, terminalID)
		return
	}
	reason := res.Reason
	if reason == "" {
		reason = "no known agent process in the pane"
	}
	if a.agentDetectReasons[terminalID] == reason {
		return
	}
	a.agentDetectReasons[terminalID] = reason
	logAgentEvent("agent_detection_none", fmt.Sprintf("terminal %d (%s, %s): %s", terminalID, terminalType, res.Source, reason))
}

// ForgetDetectionOutcome drops the pane's entry when the pane closes.
func (a *Service) ForgetDetectionOutcome(terminalID int) {
	if a == nil {
		return
	}
	a.agentMu.Lock()
	defer a.agentMu.Unlock()
	delete(a.agentDetectReasons, terminalID)
}
