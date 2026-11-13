package main

import (
	"fmt"
	"sort"
	"strings"

	"mimir/aiflow"
	"mimir/dlp"
)

// identifierMask applies dlp masking to one AI request when the provider's
// policy calls for it. A provider that may see sensitive context (a local
// model) gets a nil mask, on which every method is a no-op, so the call
// sites read the same for both kinds of provider.
//
// The switch is the existing AllowSensitiveContext policy rather than a new
// setting: a provider that must not see secret-looking lines must not see
// which hosts the user works on either.
type identifierMask struct {
	mapping *dlp.Mapping
}

func newIdentifierMask(policy aiflow.ProviderPolicyConfig) *identifierMask {
	if policy.AllowSensitiveContext {
		return nil
	}
	return &identifierMask{mapping: dlp.NewMapping()}
}

// mask replaces identifiers in text that is about to leave the machine.
func (m *identifierMask) mask(text string) string {
	if m == nil {
		return text
	}
	return m.mapping.Mask(text)
}

// annotate tells the model what the placeholders are, so it uses them
// verbatim instead of "correcting" them. Only added when something was
// masked.
func (m *identifierMask) annotate(prompt string) string {
	if m == nil || m.mapping.Empty() {
		return prompt
	}
	return prompt + "\nNote: tokens such as MIMIR_IP_1, MIMIR_IP6_1, MIMIR_MAC_1 or MIMIR_EMAIL_1 stand for real addresses that were withheld. " +
		"Use them exactly as written wherever the real value belongs; do not invent new ones and do not alter them.\n"
}

// unmask translates an answer back and returns the placeholders it could
// not resolve.
func (m *identifierMask) unmask(text string) (string, []string) {
	if m == nil {
		return text, nil
	}
	out, report := m.mapping.Unmask(text)
	return out, report.Unknown
}

// unmaskSelection translates the variables and the reason of a tool
// selection back in place and returns the unresolved placeholders.
func (m *identifierMask) unmaskSelection(selection *toolSelection) []string {
	if m == nil || selection == nil {
		return nil
	}
	seen := map[string]bool{}
	for key, value := range selection.Variables {
		restored, unknown := m.unmask(value)
		selection.Variables[key] = restored
		for _, token := range unknown {
			seen[token] = true
		}
	}
	// The reason is only shown, never executed: translate it, but an
	// unresolved placeholder in it blocks nothing.
	selection.Reason, _ = m.unmask(selection.Reason)
	unknown := make([]string, 0, len(seen))
	for token := range seen {
		unknown = append(unknown, token)
	}
	sort.Strings(unknown)
	return unknown
}

// audit adds how many identifiers were masked, per kind, to the context
// audit of the interaction log. Counts only, never the values.
func (m *identifierMask) audit(contextAudit map[string]any) map[string]any {
	if m == nil {
		return contextAudit
	}
	contextAudit["identifierMasking"] = true
	if counts := m.mapping.Counts(); len(counts) > 0 {
		contextAudit["maskedIdentifiers"] = counts
	}
	return contextAudit
}

func unknownPlaceholderWarning(unknown []string) string {
	return fmt.Sprintf("the answer contains placeholders without a real value behind them (%s); do not run it as is", strings.Join(unknown, ", "))
}
