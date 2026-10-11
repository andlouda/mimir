package executil

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// HelperTimeout bounds every out-of-band helper exec (tmux probes, remote
// discovery, WSL lookups): long enough for a slow SSH round trip, short
// enough that a hung helper cannot freeze a binding.
const HelperTimeout = 10 * time.Second

// TmuxSocket is the name of the tmux server (-L) that hosts Mimir's local
// sessions. Every out-of-band tmux call must address this socket, or it
// would talk to the user's own tmux server instead.
const TmuxSocket = "mimir"

// ValidTmuxSessionName accepts the names Mimir generates and the ones a
// user may type: letters, digits, '-' and '_'. tmux itself rewrites '.'
// and ':' in session names, so anything else could not be targeted later.
func ValidTmuxSessionName(name string) bool {
	if len(name) == 0 || len(name) > 256 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// IsWSLTerminalType reports whether a terminal type names a WSL pane: its
// processes live inside the distro, so helper commands go through wsl.exe.
func IsWSLTerminalType(terminalType string) bool {
	return strings.EqualFold(strings.TrimSpace(terminalType), "wsl")
}

// LocalTmuxShell reports whether a terminal type runs its shell under
// Mimir's tmux server on this machine (bash, zsh, or a WSL pane). SSH panes
// have their tmux on the remote host; PowerShell and cmd have none.
func LocalTmuxShell(terminalType string) bool {
	switch strings.ToLower(strings.TrimSpace(terminalType)) {
	case "bash", "zsh", "wsl":
		return true
	}
	return false
}

// ShellQuote wraps a value in POSIX single quotes so the shell passes it on
// verbatim. Safe for any input, including embedded single quotes.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// ShellQuoteIfNeeded leaves a token bare when it is made only of characters
// that no POSIX shell interprets, and single-quotes it otherwise. Use it for
// rendered scripts that a human may read (tmux option lists, resume commands).
func ShellQuoteIfNeeded(token string) string {
	if token == "" {
		return "''"
	}
	for _, r := range token {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./=:@%+,", r)) {
			return ShellQuote(token)
		}
	}
	return token
}

// TmuxArgs returns the argument vector for a tmux call against Mimir's
// socket: "tmux -L mimir args..." locally, or the same behind
// "wsl.exe --" when the sessions live in a WSL distro.
func TmuxArgs(viaWSL bool, args ...string) (string, []string) {
	tmux := append([]string{"-L", TmuxSocket}, args...)
	if viaWSL {
		return "wsl.exe", append([]string{"--", "tmux"}, tmux...)
	}
	return "tmux", tmux
}

// TmuxCommand builds the tmux call described by TmuxArgs, with the console
// window hidden on Windows. The caller runs it.
func TmuxCommand(ctx context.Context, viaWSL bool, args ...string) *exec.Cmd {
	name, argv := TmuxArgs(viaWSL, args...)
	cmd := exec.CommandContext(ctx, name, argv...)
	HideConsoleWindow(cmd)
	return cmd
}

// ShellCommand runs a POSIX shell script ("sh -c script"), locally or inside
// the default WSL distro. The script is passed as one argument, never
// re-parsed by an outer shell.
func ShellCommand(ctx context.Context, viaWSL bool, script string) *exec.Cmd {
	var cmd *exec.Cmd
	if viaWSL {
		cmd = exec.CommandContext(ctx, "wsl.exe", "--", "sh", "-c", script)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", script)
	}
	HideConsoleWindow(cmd)
	return cmd
}

// FirstLine returns the first line of a command's output, for error
// messages that quote what a helper printed.
func FirstLine(message string) string {
	if index := strings.IndexByte(message, '\n'); index >= 0 {
		return strings.TrimSpace(message[:index])
	}
	return message
}
