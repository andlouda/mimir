// Package redact strips secrets out of structured data before it is written
// to disk. It is deliberately a leaf package (no project imports) so the
// hook process, the agents package and the timeline store can all use it
// without import cycles.
//
// Two layers: keys that name a secret (api_key, authorization, password,
// ...) lose their value entirely; string values are scanned for well-known
// token shapes (Bearer, sk-, AKIA, ghp_, JWTs, PEM blocks) and KEY=value
// assignments of secret-looking names.
package redact

import (
	"encoding/json"
	"regexp"
	"sort"
)

// Placeholder replaces a redacted value.
const Placeholder = "[REDACTED]"

var (
	sensitiveKey  = regexp.MustCompile(`(?i)(api[_-]?key|authorization|bearer|cookie|pass(word|wd)?|secret|token|private[_-]?key|refresh[_-]?token|client[_-]?secret|access[_-]?key)`)
	valuePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-=/+]{10,}`),
		regexp.MustCompile(`\bsk-[A-Za-z0-9\-_]{12,}\b`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
		regexp.MustCompile(`\bglpat-[A-Za-z0-9\-_]{20,}\b`),
		regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9\-]{10,}\b`),
		regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`),
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
		// KEY=value / KEY: value / --key value for secret-looking names.
		regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:API[_-]?KEY|SECRET|TOKEN|PASSWORD|PASSWD|PRIVATE[_-]?KEY|ACCESS[_-]?KEY)[A-Z0-9_]*)\s*[=:]\s*["']?[^\s"']{6,}`),
		regexp.MustCompile(`(?i)(--?(?:password|passwd|token|api[_-]?key|secret)[= ])\S{4,}`),
	}
	valueReplace = []string{
		"Bearer " + Placeholder,
		Placeholder,
		Placeholder,
		Placeholder,
		Placeholder,
		Placeholder,
		Placeholder,
		Placeholder,
		"$1=" + Placeholder,
		"$1" + Placeholder,
	}
)

// String redacts secret shapes inside free text. The boolean reports
// whether anything changed.
func String(s string) (string, bool) {
	out := s
	for i, p := range valuePatterns {
		out = p.ReplaceAllString(out, valueReplace[i])
	}
	return out, out != s
}

// Options bound the size of a redacted value tree.
type Options struct {
	// MaxString caps every string leaf (0 = no cap). Longer strings end in
	// "...[truncated]".
	MaxString int
	// KeepLong lists keys whose string value may exceed MaxString up to
	// MaxLong (shell commands are worth keeping whole).
	KeepLong []string
	MaxLong  int
	// MaxItems caps arrays (0 = no cap); objects keep all their keys, the
	// string and depth caps bound them.
	MaxItems int
	// MaxDepth caps nesting (0 = no cap); deeper values become Placeholder.
	MaxDepth int
	// Drop lists keys removed entirely at any depth (prompt text, file
	// contents) regardless of their value.
	Drop []string
}

// Value redacts a decoded JSON value (maps, slices, strings, numbers, bools)
// in place of a copy. Keys matching the sensitive pattern lose their value.
func Value(v any, opts Options) (any, bool) {
	return value(v, "", opts, 0)
}

func value(v any, key string, opts Options, depth int) (any, bool) {
	if key != "" && sensitiveKey.MatchString(key) {
		return Placeholder, true
	}
	if opts.MaxDepth > 0 && depth > opts.MaxDepth {
		return Placeholder, true
	}
	changed := false
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			if contains(opts.Drop, k) {
				changed = true
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(keys))
		for _, k := range keys {
			nv, c := value(t[k], k, opts, depth+1)
			out[k] = nv
			changed = changed || c
		}
		return out, changed
	case []any:
		items := t
		if opts.MaxItems > 0 && len(items) > opts.MaxItems {
			items = items[:opts.MaxItems]
			changed = true
		}
		out := make([]any, len(items))
		for i, item := range items {
			nv, c := value(item, key, opts, depth+1)
			out[i] = nv
			changed = changed || c
		}
		return out, changed
	case string:
		s, c := String(t)
		limit := opts.MaxString
		if contains(opts.KeepLong, key) && opts.MaxLong > 0 {
			limit = opts.MaxLong
		}
		if limit > 0 && len(s) > limit {
			s = s[:limit] + "...[truncated]"
			c = true
		}
		return s, c
	default:
		return v, false
	}
}

// JSON redacts a raw JSON document. Invalid JSON is replaced by Placeholder.
func JSON(raw json.RawMessage, opts Options) (json.RawMessage, bool) {
	if len(raw) == 0 {
		return raw, false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return json.RawMessage(`"` + Placeholder + `"`), true
	}
	out, changed := Value(v, opts)
	b, err := json.Marshal(out)
	if err != nil {
		return json.RawMessage(`"` + Placeholder + `"`), true
	}
	return b, changed
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
