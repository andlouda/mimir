package aiflow

import (
	"regexp"
	"strings"
)

type SanitizedContext struct {
	Value      string            `json:"value"`
	Redactions []string          `json:"redactions,omitempty"`
	Audit      SanitizationAudit `json:"audit"`
}

type SanitizationAudit struct {
	RawChars              int            `json:"rawChars"`
	NormalizedChars       int            `json:"normalizedChars"`
	SanitizedChars        int            `json:"sanitizedChars"`
	MaxChars              int            `json:"maxChars"`
	Truncated             bool           `json:"truncated"`
	AllowSensitiveContext bool           `json:"allowSensitiveContext"`
	RedactionCounts       map[string]int `json:"redactionCounts,omitempty"`
	RemovedLineCounts     map[string]int `json:"removedLineCounts,omitempty"`
}

var (
	// csiSeqPattern matches CSI sequences: ESC [ params intermediates final.
	// Covers private modes such as cursor hide/show (ESC[?25l / ESC[?25h) and
	// intermediate bytes — not just the narrow ESC[<digits><letter> form.
	csiSeqPattern = regexp.MustCompile(`\x1b\[[0-9;:<=>?]*[ -/]*[@-~]`)
	// oscSeqPattern matches OSC sequences: ESC ] ... terminated by BEL or ST
	// (ESC \). Covers window-title changes (ESC]0;...BEL) and similar.
	oscSeqPattern = regexp.MustCompile("\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)")
	// escSeqPattern matches other two/three-byte escape sequences (charset
	// designations like ESC(B, ESC=, keypad modes, etc.).
	escSeqPattern = regexp.MustCompile(`\x1b[ -/]*[0-~]`)
	// ctrlCharPattern matches residual C0 control characters and DEL, keeping
	// only tab (\x09) and newline (\x0a).
	ctrlCharPattern = regexp.MustCompile("[\x00-\x08\x0b-\x1f\x7f]")
)

var repeatedWhitespacePattern = regexp.MustCompile(`[ \t]{2,}`)

// stripTerminalControl removes escape sequences from raw terminal output before
// it is sent to an AI provider. Order matters: OSC and CSI are removed before
// the generic ESC pattern, which would otherwise consume only their leading
// bytes and leave the payload (e.g. a window title) behind.
func stripTerminalControl(s string) string {
	s = oscSeqPattern.ReplaceAllString(s, "")
	s = csiSeqPattern.ReplaceAllString(s, "")
	s = escSeqPattern.ReplaceAllString(s, "")
	return s
}

var secretRedactors = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{
		name:    "openai_api_key",
		pattern: regexp.MustCompile(`\bsk-[A-Za-z0-9\-_]{12,}\b`),
	},
	{
		name:    "bearer_token",
		pattern: regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-=/+]{10,}`),
	},
	{
		name:    "authorization_header",
		pattern: regexp.MustCompile(`(?im)^Authorization:\s*[^\r\n]+`),
	},
	{
		name:    "private_key_block",
		pattern: regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`),
	},
	{
		name:    "session_cookie",
		pattern: regexp.MustCompile(`(?i)\b(cookie|session(id)?|jwt|refresh_token)\s*[:=]\s*[^\s\r\n;]+`),
	},
	{
		name:    "env_secret",
		pattern: regexp.MustCompile(`(?im)\b([A-Z0-9_]*(KEY|TOKEN|SECRET|PASSWORD|COOKIE)[A-Z0-9_]*)=([^\s\r\n]+)`),
	},
	{
		name:    "kubeconfig_token",
		pattern: regexp.MustCompile(`(?im)^\s*(token|client-key-data|client-certificate-data):\s*[^\r\n]+`),
	},
}

var strictSensitiveLinePatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{
		name:    "secret_path",
		pattern: regexp.MustCompile(`(?i)(\.ssh/|\.gnupg/|\.aws/|\.kube/config|\.pem\b|id_rsa\b|id_ed25519\b)`),
	},
	{
		name:    "shell_history_noise",
		pattern: regexp.MustCompile(`(?i)^\s*(history|fc\s+-l)\b`),
	},
	{
		name:    "secret_keyword_line",
		pattern: regexp.MustCompile(`(?i)\b(password|passwd|secret|token|authorization|cookie)\b`),
	},
}

func SanitizeTerminalContext(raw string, policy ProviderPolicyConfig) SanitizedContext {
	audit := SanitizationAudit{
		RawChars:              len(raw),
		AllowSensitiveContext: policy.AllowSensitiveContext,
		RedactionCounts:       map[string]int{},
		RemovedLineCounts:     map[string]int{},
	}
	value := stripTerminalControl(raw)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = ctrlCharPattern.ReplaceAllString(value, "")
	audit.NormalizedChars = len(value)

	redactions := make([]string, 0)
	for _, redactor := range secretRedactors {
		count := len(redactor.pattern.FindAllStringIndex(value, -1))
		if count > 0 {
			redactions = append(redactions, redactor.name)
			audit.RedactionCounts[redactor.name] += count
			value = redactor.pattern.ReplaceAllString(value, "[REDACTED]")
		}
	}

	if !policy.AllowSensitiveContext {
		lines := strings.Split(value, "\n")
		filtered := make([]string, 0, len(lines))
		for _, line := range lines {
			line = repeatedWhitespacePattern.ReplaceAllString(strings.TrimRight(line, " \t"), " ")
			skip := false
			for _, pattern := range strictSensitiveLinePatterns {
				if pattern.pattern.MatchString(line) {
					redactions = append(redactions, pattern.name)
					audit.RemovedLineCounts[pattern.name]++
					skip = true
					break
				}
			}
			if !skip {
				filtered = append(filtered, line)
			}
		}
		value = strings.Join(filtered, "\n")
	}

	maxChars := policy.MaxContextChars
	if maxChars <= 0 {
		maxChars = 4000
	}
	audit.MaxChars = maxChars
	if len(value) > maxChars {
		value = value[len(value)-maxChars:]
		audit.Truncated = true
	}

	value = strings.TrimSpace(value)
	audit.SanitizedChars = len(value)
	return SanitizedContext{
		Value:      value,
		Redactions: dedupeStrings(redactions),
		Audit:      audit.withoutEmptyCounts(),
	}
}

func (a SanitizationAudit) withoutEmptyCounts() SanitizationAudit {
	if len(a.RedactionCounts) == 0 {
		a.RedactionCounts = nil
	}
	if len(a.RemovedLineCounts) == 0 {
		a.RemovedLineCounts = nil
	}
	return a
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
