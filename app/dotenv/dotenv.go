// Package dotenv parses .env file contents into key/value entries for the
// secure viewer. It never logs or prints values; callers are responsible for
// keeping the returned values out of logs, recordings and scrollback.
package dotenv

import "strings"

// MaxFileSize caps how much of a .env file is read. Real .env files are small;
// the cap protects against accidentally slurping a huge file into memory and
// the UI.
const MaxFileSize = 256 * 1024

// Entry is a single parsed assignment. Value is the decoded plaintext; the
// viewer masks it until the user explicitly reveals it.
type Entry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Parse turns raw .env file content into ordered entries. It tolerates the
// common dotenv dialect: blank lines, `#` comments, an optional `export `
// prefix, and single- or double-quoted values. Malformed lines are skipped
// rather than aborting the parse. The parser is intentionally forgiving and
// never returns an error so a single bad line cannot hide the rest of the file.
func Parse(content string) []Entry {
	content = strings.TrimPrefix(content, "\ufeff") // strip UTF-8 BOM
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	var entries []Entry
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)

		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if !isValidKey(key) {
			continue
		}
		value := parseValue(strings.TrimSpace(line[eq+1:]))
		entries = append(entries, Entry{Key: key, Value: value})
	}
	return entries
}

// isValidKey accepts the shell-style identifiers used by .env files
// ([A-Za-z_][A-Za-z0-9_.]*) and rejects anything with spaces or odd shapes so
// that a stray malformed line is not surfaced as a bogus secret.
func isValidKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			// always allowed
		case (r >= '0' && r <= '9') || r == '.':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// parseValue decodes a single value token. Double-quoted values support the
// usual escape sequences; single-quoted values are literal; unquoted values
// have trailing inline comments and whitespace stripped.
func parseValue(raw string) string {
	if raw == "" {
		return ""
	}
	switch raw[0] {
	case '"':
		return parseDoubleQuoted(raw)
	case '\'':
		return parseSingleQuoted(raw)
	default:
		return parseUnquoted(raw)
	}
}

func parseDoubleQuoted(raw string) string {
	var b strings.Builder
	escaped := false
	for i := 1; i < len(raw); i++ {
		c := raw[i]
		if escaped {
			switch c {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(c)
			}
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '"':
			return b.String() // closing quote ends the value
		default:
			b.WriteByte(c)
		}
	}
	return b.String() // unterminated quote: return what we have
}

func parseSingleQuoted(raw string) string {
	if end := strings.IndexByte(raw[1:], '\''); end >= 0 {
		return raw[1 : 1+end]
	}
	return raw[1:] // unterminated quote: return the remainder literally
}

func parseUnquoted(raw string) string {
	// An inline comment starts at a `#` that is preceded by whitespace.
	for i := 1; i < len(raw); i++ {
		if raw[i] == '#' && (raw[i-1] == ' ' || raw[i-1] == '\t') {
			raw = raw[:i]
			break
		}
	}
	return strings.TrimSpace(raw)
}
