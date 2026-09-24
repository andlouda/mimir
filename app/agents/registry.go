// Package agents detects coding agents (Claude Code, Codex, ...) running inside
// Mimir terminals and reads the transcripts those agents write locally, so the
// UI can show agent state and offer exact, unwrapped copies of their output.
package agents

import "strings"

// Kind identifies a supported agent CLI.
type Kind string

const (
	KindClaude   Kind = "claude"
	KindCodex    Kind = "codex"
	KindGemini   Kind = "gemini"
	KindOpenCode Kind = "opencode"
	KindAider    Kind = "aider"
	KindHermes   Kind = "hermes"
)

// Descriptor describes one supported agent.
type Descriptor struct {
	Kind Kind `json:"kind"`
	// Label is the human-readable name shown in the UI.
	Label string `json:"label"`
	// Short is the compact code for the sidebar row and the pane badge.
	Short string `json:"short"`
	// Binaries are the executable base names that identify the agent in a
	// process list (case-insensitive; a trailing .exe/.cmd is ignored).
	Binaries []string `json:"-"`
	// Transcripts reports whether Mimir can read this agent's session files.
	Transcripts bool `json:"transcripts"`
}

var registry = []Descriptor{
	{Kind: KindClaude, Label: "Claude", Short: "C", Binaries: []string{"claude"}, Transcripts: true},
	{Kind: KindCodex, Label: "Codex", Short: "cx", Binaries: []string{"codex"}, Transcripts: true},
	{Kind: KindGemini, Label: "Gemini CLI", Short: "g", Binaries: []string{"gemini"}, Transcripts: false},
	// OpenCode transcripts come from its local SQLite database (not over SSH).
	{Kind: KindOpenCode, Label: "OpenCode", Short: "oc", Binaries: []string{"opencode"}, Transcripts: true},
	{Kind: KindAider, Label: "Aider", Short: "a", Binaries: []string{"aider"}, Transcripts: false},
	{Kind: KindHermes, Label: "Hermes", Short: "h", Binaries: []string{"hermes"}, Transcripts: false},
}

// Registry returns the supported agents.
func Registry() []Descriptor {
	out := make([]Descriptor, len(registry))
	copy(out, registry)
	return out
}

// Lookup returns the descriptor for a kind.
func Lookup(kind Kind) (Descriptor, bool) {
	for _, d := range registry {
		if d.Kind == kind {
			return d, true
		}
	}
	return Descriptor{}, false
}

// descriptorForBinary matches an executable base name against the registry.
func descriptorForBinary(base string) (Descriptor, bool) {
	base = strings.ToLower(strings.TrimSpace(base))
	for _, suffix := range []string{".exe", ".cmd", ".bat", ".js", ".mjs"} {
		base = strings.TrimSuffix(base, suffix)
	}
	if base == "" {
		return Descriptor{}, false
	}
	for _, d := range registry {
		for _, b := range d.Binaries {
			if base == b {
				return d, true
			}
		}
	}
	return Descriptor{}, false
}
