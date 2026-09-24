package agents

import (
	"regexp"
	"strings"
	"unicode"
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
		for len([]rune(line)) == width && i+1 < len(lines) && joinable(line, lines[i+1]) {
			line += strings.TrimLeft(lines[i+1], " ")
			i++
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// joinable reports whether a full-width row looks like a token that was cut
// mid-way: it must end in a word character and the next row must start with
// one. Rules drawn from box characters ("────") and rows ending in a space
// are complete on their own and never joined.
func joinable(line, next string) bool {
	runes := []rune(line)
	if len(runes) == 0 {
		return false
	}
	last := runes[len(runes)-1]
	nextTrim := strings.TrimLeft(next, " ")
	if nextTrim == "" {
		return false
	}
	first := []rune(nextTrim)[0]
	return isTokenRune(last) && isTokenRune(first)
}

func isTokenRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	return strings.ContainsRune("/._~:-=&?%+@#", r)
}

// TrimToAgentStart cuts pane text down to the last agent session: everything
// before the last shell prompt line that launched the agent (e.g. "~ $ claude")
// is dropped. When no such line exists, the last maxLines lines are kept.
// The number of dropped lines is returned so the UI can say so.
func TrimToAgentStart(text string, kind Kind, maxLines int) (string, int) {
	lines := strings.Split(text, "\n")
	start := -1
	if d, ok := Lookup(kind); ok {
		// The newest start wins: either the prompt line that launched the
		// binary or the agent's own start banner (the launch line is often
		// gone when a full-screen TUI redraws, the banner never is).
		for i := len(lines) - 1; i >= 0; i-- {
			if promptLaunches(lines[i], d.Binaries) || bannerStarts(lines[i], kind) {
				start = i
				break
			}
		}
		// A banner spans several lines: back up to its first line, and to
		// the launch line right above it when there is one.
		for start > 0 && bannerStarts(lines[start-1], kind) {
			start--
		}
		if start > 0 && promptLaunches(lines[start-1], d.Binaries) {
			start--
		}
	}
	if start < 0 && maxLines > 0 && len(lines) > maxLines {
		start = len(lines) - maxLines
	}
	if start <= 0 {
		return text, 0
	}
	return strings.Join(lines[start:], "\n"), start
}

// bannerStarts reports whether a line is the first line of an agent's start
// banner.
func bannerStarts(line string, kind Kind) bool {
	t := strings.TrimSpace(line)
	switch kind {
	case KindClaude:
		return strings.HasPrefix(t, "Claude Code v") || strings.Contains(t, "▐▛███▛█")
	case KindCodex:
		return strings.Contains(t, "OpenAI Codex")
	case KindOpenCode:
		return strings.HasPrefix(t, "█▀▀█ █▀▀█ █▀▀▀") || strings.HasPrefix(strings.ToLower(t), "opencode v")
	}
	return false
}

// promptLaunches reports whether a line looks like a shell prompt that ran one
// of the binaries: the prompt marker ($, %, #, ❯, >) followed by the binary as
// the command word.
func promptLaunches(line string, binaries []string) bool {
	trimmed := strings.TrimSpace(line)
	for _, marker := range []string{"$ ", "% ", "# ", "❯ ", "> "} {
		idx := strings.LastIndex(trimmed, marker)
		if idx < 0 {
			continue
		}
		cmd := strings.TrimSpace(trimmed[idx+len(marker):])
		word := cmd
		if sp := strings.IndexByte(cmd, ' '); sp >= 0 {
			word = cmd[:sp]
		}
		base := word
		if slash := strings.LastIndexAny(word, "/\\"); slash >= 0 {
			base = word[slash+1:]
		}
		for _, b := range binaries {
			if base == b {
				return true
			}
		}
	}
	return false
}

// CleanPaneText trims trailing whitespace per line and collapses the empty
// tail tmux pads the capture with.
func CleanPaneText(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := lines[:0]
	blank := 0
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			blank++
			// A full-screen layout pads with empty rows; two are enough to
			// keep the structure readable.
			if blank > 2 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
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
