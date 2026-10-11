package terminal

import (
	"regexp"
	"strings"
	"sync"
)

// The app can hand every new local shell extra environment variables (the
// agent timeline uses this to point Claude Code's telemetry at Mimir's own
// receiver). The provider is asked at launch time, so a settings change
// applies to the next shell without a restart. The terminal package does
// not know the settings themselves: main owns them.

var (
	launchEnvMu       sync.Mutex
	launchEnvProvider func() []string
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SetLaunchEnvProvider installs the source of extra KEY=VALUE pairs for new
// local shells (nil removes it).
func SetLaunchEnvProvider(fn func() []string) {
	launchEnvMu.Lock()
	defer launchEnvMu.Unlock()
	launchEnvProvider = fn
}

// launchEnv returns the extra pairs, keeping only well-formed KEY=VALUE
// entries with a valid variable name and no newlines (a bad entry must not
// break the rc file or the environment block).
func launchEnv() []string {
	launchEnvMu.Lock()
	fn := launchEnvProvider
	launchEnvMu.Unlock()
	if fn == nil {
		return nil
	}
	var out []string
	for _, kv := range fn() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !envNamePattern.MatchString(k) || strings.ContainsAny(v, "\n\r\x00") {
			continue
		}
		out = append(out, k+"="+v)
	}
	return out
}

// launchEnvExports renders the pairs as POSIX export lines for an rc file,
// so they reach shells started inside an already running tmux server too.
func launchEnvExports() string {
	env := launchEnv()
	if len(env) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Mimir: agent telemetry (Settings > Agent-Zeitleiste)\n")
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		b.WriteString("export " + k + "=" + singleQuote(v) + "\n")
	}
	return b.String()
}

// singleQuote wraps a value in single quotes for POSIX shells (shared by
// the Unix and Windows launch paths, so it does not live in either file).
func singleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
