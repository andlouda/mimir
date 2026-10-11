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
// wsl adds the host probe for WSL2 distros.
func launchEnvExports(wsl bool) string {
	return RenderExports(launchEnv(), wsl)
}

// endpointVar is the variable whose value differs inside WSL2.
const endpointVar = "OTEL_EXPORTER_OTLP_ENDPOINT"

var loopbackEndpoint = regexp.MustCompile(`^http://127\.0\.0\.1:(\d+)$`)

// RenderExports renders KEY=VALUE pairs as POSIX export lines. With wsl,
// a loopback endpoint becomes a probe: 127.0.0.1 when it answers (mirrored
// networking), else the distro's default gateway, which is the Windows
// host on the WSL virtual switch where Mimir listens as well.
func RenderExports(env []string, wsl bool) string {
	if len(env) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Mimir: agent telemetry (Settings > Agent-Zeitleiste)\n")
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if wsl && k == endpointVar {
			if m := loopbackEndpoint.FindStringSubmatch(v); m != nil {
				port := m[1]
				b.WriteString("if (exec 3<>/dev/tcp/127.0.0.1/" + port + ") 2>/dev/null; then __mimir_otel_host=127.0.0.1; else __mimir_otel_host=$(ip route show default 2>/dev/null | awk '/default/ {print $3; exit}'); fi\n")
				b.WriteString("export " + k + "=\"http://${__mimir_otel_host:-127.0.0.1}:" + port + "\"\n")
				b.WriteString("unset __mimir_otel_host\n")
				continue
			}
		}
		b.WriteString("export " + k + "=" + singleQuote(v) + "\n")
	}
	return b.String()
}

// singleQuote wraps a value in single quotes for POSIX shells (shared by
// the Unix and Windows launch paths, so it does not live in either file).
func singleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
