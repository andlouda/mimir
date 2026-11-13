// Package dlp keeps identifying values out of prompts that leave the
// machine, without making the answer useless.
//
// A Mapping replaces network identifiers (IP and MAC addresses, e-mail
// addresses) with stable placeholders before a prompt goes to a cloud
// provider: every occurrence of 10.0.3.17 becomes MIMIR_IP_1, so the model
// can still reason about which host talks to which. The mapping never leaves
// the process; Unmask puts the real values back into the answer, including
// into a command the model suggests.
//
// Both directions are deterministic string replacement. No model is involved
// in translating back: a command that will be shown to the user for
// execution must contain exactly the values that were masked, not a model's
// rewrite of them. Placeholders the mapping does not know (invented or
// mangled by the model) are left in place and reported, so the caller can
// warn instead of handing out a command with a made-up target.
package dlp

import (
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kind names what a placeholder stands for; it is part of the placeholder.
type Kind string

const (
	KindIPv4  Kind = "IP"
	KindIPv6  Kind = "IP6"
	KindMAC   Kind = "MAC"
	KindEmail Kind = "EMAIL"
)

// placeholderPrefix makes placeholders recognisable and shell-safe: only
// word characters, so one that survives into a command is a harmless word
// rather than shell syntax.
const placeholderPrefix = "MIMIR_"

var (
	placeholderPattern = regexp.MustCompile(`\bMIMIR_(?:IP6|IP|MAC|EMAIL|HOST|USER|NAME)_[0-9]+\b`)

	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9\-]+(?:\.[A-Za-z0-9\-]+)*\.[A-Za-z]{2,}`)
	macPattern   = regexp.MustCompile(`(?:[0-9A-Fa-f]{2}[:\-]){5}[0-9A-Fa-f]{2}`)
	// ipv6Candidate is deliberately loose; netip decides what is an address.
	ipv6Candidate = regexp.MustCompile(`(?:[0-9A-Fa-f]{0,4}:){2,7}(?:[0-9]{1,3}(?:\.[0-9]{1,3}){3}|[0-9A-Fa-f]{0,4})`)
	ipv4Candidate = regexp.MustCompile(`[0-9]{1,3}(?:\.[0-9]{1,3}){3}`)
)

// Mapping holds the placeholders of one request. It is not safe for
// concurrent use; create one per prompt/answer round trip.
type Mapping struct {
	tokenOf map[string]string // canonical value → placeholder
	valueOf map[string]string // placeholder → value as first seen
	next    map[Kind]int
	counts  map[Kind]int    // occurrences replaced, per kind
	terms   map[string]Kind // learned literal terms, see terms.go
}

// NewMapping returns an empty mapping.
func NewMapping() *Mapping {
	return &Mapping{
		tokenOf: map[string]string{},
		valueOf: map[string]string{},
		next:    map[Kind]int{},
		counts:  map[Kind]int{},
	}
}

// Mask replaces every identifier in text with its placeholder. Calling it
// for several texts of the same request (terminal output, user goal) keeps
// the placeholders consistent across them.
func (m *Mapping) Mask(text string) string {
	if text == "" {
		return text
	}
	text = m.replace(text, emailPattern, KindEmail, func(s string) (string, bool) {
		return strings.ToLower(s), true
	})
	text = m.replace(text, macPattern, KindMAC, func(s string) (string, bool) {
		return strings.ToLower(strings.ReplaceAll(s, "-", ":")), true
	})
	text = m.replace(text, ipv6Candidate, KindIPv6, canonicalIPv6)
	text = m.replace(text, ipv4Candidate, KindIPv4, canonicalIPv4)
	return m.maskTerms(text)
}

// canonicalIPv4 accepts addresses that identify a host or network. Loopback,
// the unspecified address and netmasks say nothing about the environment and
// stay readable.
func canonicalIPv4(s string) (string, bool) {
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is4() {
		return "", false
	}
	if addr.IsLoopback() || addr.IsUnspecified() || strings.HasPrefix(s, "255.") {
		return "", false
	}
	return addr.String(), true
}

func canonicalIPv6(s string) (string, bool) {
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is6() {
		return "", false
	}
	if addr.IsLoopback() || addr.IsUnspecified() {
		return "", false
	}
	// A valid address needs at least one real group; "a::" or "::b" is far
	// more often a scope operator in code than a host.
	if len(strings.ReplaceAll(s, ":", "")) < 3 {
		return "", false
	}
	return addr.String(), true
}

// replace swaps every accepted match of pattern for its placeholder. A match
// glued to further word characters is part of something else (a version
// string, an identifier) and is left alone.
func (m *Mapping) replace(text string, pattern *regexp.Regexp, kind Kind, canonical func(string) (string, bool)) string {
	matches := pattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, loc := range matches {
		start, end := loc[0], loc[1]
		if start > 0 && isJoiner(text[start-1], kind) {
			continue
		}
		if end < len(text) && isJoiner(text[end], kind) {
			continue
		}
		key, ok := canonical(text[start:end])
		if !ok {
			continue
		}
		out.WriteString(text[last:start])
		out.WriteString(m.token(kind, key, text[start:end]))
		m.counts[kind]++
		last = end
	}
	out.WriteString(text[last:])
	return out.String()
}

// isJoiner reports whether b next to a match means the match is only a part
// of a longer token.
func isJoiner(b byte, kind Kind) bool {
	if b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_' {
		return true
	}
	switch kind {
	case KindIPv6, KindMAC:
		return b == ':'
	case KindIPv4:
		// "1.2.3.4.5" and "1.2.3.4-beta" are versions, not addresses. A
		// trailing sentence dot is indistinguishable and costs one address.
		return b == '.' || b == '-'
	}
	return false
}

func (m *Mapping) token(kind Kind, key, original string) string {
	mapKey := string(kind) + "\x00" + key
	if token, ok := m.tokenOf[mapKey]; ok {
		return token
	}
	m.next[kind]++
	token := placeholderPrefix + string(kind) + "_" + strconv.Itoa(m.next[kind])
	m.tokenOf[mapKey] = token
	m.valueOf[token] = original
	return token
}

// UnmaskReport says what Unmask did.
type UnmaskReport struct {
	// Restored counts the placeholders replaced by their value.
	Restored int
	// Unknown lists placeholders the mapping never handed out; they are
	// still in the text.
	Unknown []string
}

// Unmask puts the real values back for every placeholder of this mapping.
func (m *Mapping) Unmask(text string) (string, UnmaskReport) {
	var report UnmaskReport
	unknown := map[string]bool{}
	out := placeholderPattern.ReplaceAllStringFunc(text, func(token string) string {
		if value, ok := m.valueOf[token]; ok {
			report.Restored++
			return value
		}
		unknown[token] = true
		return token
	})
	for token := range unknown {
		report.Unknown = append(report.Unknown, token)
	}
	sort.Strings(report.Unknown)
	return out, report
}

// Counts returns how many occurrences were masked per kind, for the audit
// log. It never contains the values themselves.
func (m *Mapping) Counts() map[string]int {
	if len(m.counts) == 0 {
		return nil
	}
	out := make(map[string]int, len(m.counts))
	for kind, n := range m.counts {
		out[string(kind)] = n
	}
	return out
}

// Empty reports whether nothing was masked.
func (m *Mapping) Empty() bool {
	return len(m.valueOf) == 0
}
