package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"mimir/aiflow"
	"mimir/dlp"
)

// The agent bridge: text that travels between a pane and the pane an agent
// (Claude Code, Codex, ...) runs in.
//
// Towards the agent, a selection is scrubbed of secrets and its network
// identifiers are replaced by placeholders. Back from the agent, a command
// the user selected is translated to the real values before it is pasted
// into another pane. Both are explicit user actions on a selection; nothing
// is sent or executed on the user's behalf, the paste never presses Enter.
//
// Unlike a single AskAI request, a conversation with an agent runs over many
// messages, so each agent pane keeps one mapping for as long as the pane
// lives: MIMIR_IP_1 means the same host in every message. The mapping stays
// in memory and is dropped with the pane.

type agentMaskStore struct {
	mu     sync.Mutex
	byPane map[int]*dlp.Mapping
}

func (s *agentMaskStore) mapping(agentTerminalID int) *dlp.Mapping {
	if s.byPane == nil {
		s.byPane = map[int]*dlp.Mapping{}
	}
	m, ok := s.byPane[agentTerminalID]
	if !ok {
		m = dlp.NewMapping()
		s.byPane[agentTerminalID] = m
	}
	return m
}

func (s *agentMaskStore) forget(agentTerminalID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byPane, agentTerminalID)
}

type agentMaskResult struct {
	Text string `json:"text"`
	// Masked counts the identifiers replaced in this text, per kind.
	Masked map[string]int `json:"masked,omitempty"`
}

type agentUnmaskResult struct {
	Text     string `json:"text"`
	Restored int    `json:"restored"`
	// Unknown lists placeholders this agent pane's mapping never handed
	// out; the frontend refuses to paste a text that still has any.
	Unknown []string `json:"unknown,omitempty"`
}

// MaskTextForAgentJSON prepares text for the agent in the given pane:
// secrets become [REDACTED] (not reversible), network identifiers become
// placeholders of that pane's mapping.
func (a *App) MaskTextForAgentJSON(agentTerminalID int, text string) (string, error) {
	// Stage one, patterns. Its result is what the local model gets to see.
	a.agentMasks.mu.Lock()
	mapping := a.agentMasks.mapping(agentTerminalID)
	before := mapping.Counts()
	masked := mapping.Mask(aiflow.ScrubSecrets(text))
	a.agentMasks.mu.Unlock()

	// Stage two, the local model names further terms; the call runs
	// without the lock. If it is switched on and fails, nothing is sent.
	if err := a.learnTerms(mapping, &a.agentMasks.mu, masked); err != nil {
		return "", err
	}
	a.agentMasks.mu.Lock()
	masked = mapping.Mask(masked)
	after := mapping.Counts()
	a.agentMasks.mu.Unlock()

	delta := map[string]int{}
	for kind, n := range after {
		if d := n - before[kind]; d > 0 {
			delta[kind] = d
		}
	}
	a.logAIInteraction(AIInteractionLogEntry{
		Mode:         "send_to_agent_masked",
		TerminalID:   agentTerminalID,
		ContextAudit: map[string]any{"identifierMasking": true, "maskedIdentifiers": delta, "chars": len(masked)},
	})
	return encodeJSON(agentMaskResult{Text: masked, Masked: delta})
}

// UnmaskTextFromAgentJSON translates text taken from the agent's pane back
// to the real values of that pane's mapping.
func (a *App) UnmaskTextFromAgentJSON(agentTerminalID int, text string) (string, error) {
	a.agentMasks.mu.Lock()
	restored, report := a.agentMasks.mapping(agentTerminalID).Unmask(text)
	a.agentMasks.mu.Unlock()
	return encodeJSON(agentUnmaskResult{Text: restored, Restored: report.Restored, Unknown: report.Unknown})
}

func encodeJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode result: %w", err)
	}
	return string(data), nil
}
