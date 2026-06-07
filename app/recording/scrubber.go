package recording

import "regexp"

var scrubPatterns = []*regexp.Regexp{
	// Bearer tokens
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-=/+]{10,}`),
	// OpenAI keys
	regexp.MustCompile(`\bsk-[A-Za-z0-9\-_]{12,}\b`),
	// AWS access keys
	regexp.MustCompile(`\bAKIA[A-Z0-9]{16}\b`),
	// AWS secret keys (40 chars base64-like after a separator)
	regexp.MustCompile(`(?i)(aws_secret_access_key|secret_access_key)\s*[=:]\s*[A-Za-z0-9/+=]{20,}`),
	// Generic key=value secrets
	regexp.MustCompile(`(?i)(api[_-]?key|password|secret|token|private[_-]?key|refresh[_-]?token)\s*[=:]\s*\S+`),
	// export VAR=value for sensitive vars
	regexp.MustCompile(`(?i)export\s+(API[_-]?KEY|SECRET|TOKEN|PASSWORD|PRIVATE[_-]?KEY)\s*=\s*\S+`),
	// GitHub tokens (classic) and fine-grained PATs
	regexp.MustCompile(`\b(ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9_]{36,}\b`),
	regexp.MustCompile(`\bgithub_pat_[0-9A-Za-z_]{22,}\b`),
	// JWTs (three base64url segments)
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
	// Slack tokens
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`),
	// Google API keys
	regexp.MustCompile(`\bAIza[0-9A-Za-z\-_]{35}\b`),
	// Google OAuth client secrets
	regexp.MustCompile(`\bGOCSPX-[0-9A-Za-z_\-]{20,}\b`),
	// Stripe live/restricted keys
	regexp.MustCompile(`\b(sk|rk)_live_[0-9A-Za-z]{16,}\b`),
}

// NOTE: pattern-based scrubbing is best-effort defense-in-depth, not a
// guarantee. It cannot detect free-form secrets (e.g. a password typed at a
// prompt). See docs/security-notes.md and the input-recording opt-in.

// ScrubOutput replaces sensitive patterns in data with [REDACTED].
func ScrubOutput(data string) string {
	for _, pat := range scrubPatterns {
		data = pat.ReplaceAllString(data, "[REDACTED]")
	}
	return data
}
