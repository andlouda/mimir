package agents

import (
	"regexp"
	"strings"
)

// PaneSource marks a transcript that was assembled from the tmux pane
// contents instead of an agent session file.
const (
	SourceFile = "file"
	SourceTmux = "tmux"
)

// JoinFullWidthRows re-joins lines that an application hard-wrapped at the
// pane width: a row that fills every column and is followed by a non-empty
// row is glued to it. tmux's own soft wraps are already joined by
// `capture-pane -J`; this covers agents that break long tokens (URLs, paths)
// themselves. It cannot recover breaks placed at word boundaries, so the
// result is best-effort and flagged as such in the UI.
func JoinFullWidthRows(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		for len([]rune(line)) == width && i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" && !strings.HasSuffix(line, " ") {
			line += strings.TrimLeft(lines[i+1], " ")
			i++
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// CleanPaneText trims trailing whitespace per line and collapses the empty
// tail tmux pads the capture with.
func CleanPaneText(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// PaneTranscript wraps captured pane text as a single-message transcript so
// the panel can show it when no session file is available (unsupported
// agent, changed file format, cleared history).
func PaneTranscript(kind Kind, label, cwd, text string) Transcript {
	if label == "" {
		if d, ok := Lookup(kind); ok {
			label = d.Label
		}
	}
	return Transcript{
		Kind:     kind,
		Label:    label,
		Cwd:      cwd,
		Source:   SourceTmux,
		Messages: []Message{{Role: "assistant", Text: text}},
	}
}

var tokenPattern = regexp.MustCompile(`[\p{L}\p{N}_./-]{4,}`)

// tokenSet extracts distinctive lowercase tokens from text. Words shorter
// than four characters are skipped: they match anywhere and carry no signal.
func tokenSet(text string, limit int) map[string]struct{} {
	set := make(map[string]struct{})
	for _, tok := range tokenPattern.FindAllString(strings.ToLower(text), -1) {
		set[tok] = struct{}{}
		if limit > 0 && len(set) >= limit {
			break
		}
	}
	return set
}

// MatchScore reports which fraction of the distinctive tokens of message
// also occur in paneText. It is used to confirm that a session file belongs
// to the pane being looked at: the agent's last answer is rendered on screen,
// so most of its words must be visible in the capture. Tokens are compared
// individually, which makes the check robust against the agent's own line
// wrapping and cursor-positioned rendering.
func MatchScore(message, paneText string) (score float64, tokens int) {
	const maxTokens = 250
	// The tail of a long answer is what is most likely still on screen.
	if len(message) > 4000 {
		message = message[len(message)-4000:]
	}
	want := tokenSet(message, maxTokens)
	if len(want) == 0 {
		return 0, 0
	}
	have := tokenSet(paneText, 0)
	hits := 0
	for tok := range want {
		if _, ok := have[tok]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(want)), len(want)
}

const (
	// verifyMinTokens is the minimum number of distinctive tokens a message
	// needs before a comparison is considered meaningful.
	verifyMinTokens = 5
	// verifyThreshold is the fraction of tokens that must be on screen.
	verifyThreshold = 0.5
)

// Verified reports whether the score/token pair counts as a confirmation.
func Verified(score float64, tokens int) bool {
	return tokens >= verifyMinTokens && score >= verifyThreshold
}
