package agentsvc

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"mimir/timeline"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// agentTimelinePayload is what GetAgentTimelineJSON returns and what the
// live event "agent-timeline-<terminalID>" carries (then with only the new
// events).
type agentTimelinePayload struct {
	Enabled    bool             `json:"enabled"`
	TerminalID int              `json:"terminalId"`
	SessionID  string           `json:"sessionId,omitempty"`
	Path       string           `json:"path,omitempty"`
	Events     []timeline.Event `json:"events"`
	NextSeq    uint64           `json:"nextSeq"`
}

const (
	timelineBatchWindow = 250 * time.Millisecond
	timelineBatchMax    = 200
	timelineDefaultLoad = 500
	timelineMaxLoad     = 5000
)

// startTimelineEmitter wires the store to the frontend: appended events of
// a bound pane arrive as "agent-timeline-<id>" in small batches.
func (a *Service) startTimelineEmitter() {
	if a.timeline == nil || a.ctx == nil {
		return
	}
	batcher := timeline.NewBatcher(timelineBatchWindow, timelineBatchMax, func(terminalID int, events []timeline.Event) {
		payload := agentTimelinePayload{Enabled: true, TerminalID: terminalID, SessionID: events[0].SessionID, Events: events, NextSeq: events[len(events)-1].Seq}
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		wailsruntime.EventsEmit(a.ctx, fmt.Sprintf("agent-timeline-%d", terminalID), string(data))
	})
	a.timeline.Subscribe(batcher.Add)
}

// timelineSessionFor resolves the session a pane's timeline belongs to: the
// hook binding first, else the pinned or bound transcript file.
func (a *Service) timelineSessionFor(terminalID int) string {
	if a.timeline == nil {
		return ""
	}
	if s := a.timeline.SessionFor(terminalID); s != "" {
		return s
	}
	if st, ok := a.rememberedAgent(terminalID); ok {
		if file := st.effectiveSessionFile(); file != "" {
			return timeline.SessionIDFromTranscript(file)
		}
	}
	return ""
}

// GetAgentTimelineJSON returns the recent timeline events of a pane's agent
// session (Seq > afterSeq, newest limit of them; limit 0 = 500).
func (a *Service) GetAgentTimelineJSON(terminalID int, afterSeq int, limit int) (string, error) {
	payload := agentTimelinePayload{Enabled: a.telemetrySettings().Enabled, TerminalID: terminalID, Events: []timeline.Event{}}
	if limit <= 0 {
		limit = timelineDefaultLoad
	}
	if limit > timelineMaxLoad {
		limit = timelineMaxLoad
	}
	if afterSeq < 0 {
		afterSeq = 0
	}
	session := a.timelineSessionFor(terminalID)
	if session != "" && timeline.ValidSessionID(session) {
		payload.SessionID = session
		if p, err := a.timeline.Path(session); err == nil {
			if _, statErr := os.Stat(p); statErr == nil {
				payload.Path = p
			}
		}
		events, err := a.timeline.Recent(session, uint64(afterSeq), limit)
		if err != nil {
			return "", err
		}
		payload.Events = events
		payload.NextSeq = a.timeline.LastSeq(session)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// GetAgentTimelinePath is the JSONL file of a pane's session ("" when none
// has been written yet).
func (a *Service) GetAgentTimelinePath(terminalID int) (string, error) {
	session := a.timelineSessionFor(terminalID)
	if session == "" {
		return "", nil
	}
	p, err := a.timeline.Path(session)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err != nil {
		return "", nil
	}
	return p, nil
}

// OpenAgentTimelineFolder shows the timeline directory in the file manager.
func (a *Service) OpenAgentTimelineFolder() error {
	if a.timeline == nil {
		return fmt.Errorf("timeline store unavailable")
	}
	dir := a.timeline.Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return a.deps.OpenPath(dir)
}
