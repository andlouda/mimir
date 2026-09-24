package terminal

import "strings"

// tmux integration modes. tmux keeps sessions alive across app restarts and
// SSH drops; how much of it the user sees is a preference:
//
//   - invisible: tmux is a persistence layer only. Its mouse support stays
//     off, so selecting, copying and the context menu are handled by xterm
//     exactly like in a terminal without tmux. Wheel scrolling still reaches
//     tmux's history: the frontend sends Shift+PageUp/PageDown key codes that
//     are bound to copy-mode scrolling (see TmuxOptionCommands).
//   - classic: tmux owns the mouse. Drags select in tmux copy-mode, the
//     selection travels back via OSC 52 (or Mimir's buffer sync), wheel
//     scrolls through mouse events.
//   - off: local terminals start without tmux (no persistence). SSH profiles
//     keep their own per-profile switch.
const (
	TmuxModeInvisible = "invisible"
	TmuxModeClassic   = "classic"
	TmuxModeOff       = "off"
)

// NormalizeTmuxMode maps unknown or empty values to the default mode.
func NormalizeTmuxMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case TmuxModeClassic:
		return TmuxModeClassic
	case TmuxModeOff:
		return TmuxModeOff
	default:
		return TmuxModeInvisible
	}
}

// TmuxMouseEnabled reports whether tmux handles the mouse in this mode.
func TmuxMouseEnabled(mode string) bool {
	return NormalizeTmuxMode(mode) == TmuxModeClassic
}

// msOverride teaches tmux that the outer terminal accepts OSC 52 clipboard
// writes; most xterm-256color terminfo entries lack the Ms capability.
const msOverride = `,xterm*:Ms=\E]52;%p1%s;%p2%s\007`

// TmuxOptionCommands lists the tmux commands applied right after
// new-session -A, as token lists. Rendered by TmuxOptionArgs (exec argv) and
// TmuxOptionScript (POSIX shell string) so the three launch paths — local
// exec, WSL via bash -lc, SSH via sh -lc — cannot drift apart.
func TmuxOptionCommands(mode string) [][]string {
	mode = NormalizeTmuxMode(mode)
	cmds := [][]string{
		{"set", "status", "off"},
		{"set", "escape-time", "0"},
		{"set", "history-limit", "100000"},
		{"set", "prefix", "None"},
		{"set", "prefix2", "None"},
		// "on": tmux forwards its own copy-mode selections AND OSC 52
		// writes from programs in the pane to the outer terminal, and keeps
		// a copy in its paste buffer. "external" would drop the latter, which
		// broke copying inside a nested tmux (e.g. a container whose shell
		// starts its own tmux): its selections never left the inner server.
		// Plain (non-tmux) terminals accept OSC 52 writes anyway; reads stay
		// denied in the frontend provider.
		{"set", "-s", "set-clipboard", "on"},
		{"set", "-ga", "terminal-overrides", msOverride},
		// Mimir draws its own context menu.
		{"unbind-key", "-n", "MouseDown3Pane"},
		{"unbind-key", "-n", "M-MouseDown3Pane"},
	}
	if mode == TmuxModeClassic {
		cmds = append(cmds,
			[]string{"set", "mouse", "on"},
			// Finer wheel steps (default is 5 lines per wheel event).
			[]string{"bind-key", "-T", "copy-mode", "WheelUpPane", "send-keys", "-N3", "-X", "scroll-up"},
			[]string{"bind-key", "-T", "copy-mode", "WheelDownPane", "send-keys", "-N3", "-X", "scroll-down"},
			[]string{"bind-key", "-T", "copy-mode-vi", "WheelUpPane", "send-keys", "-N3", "-X", "scroll-up"},
			[]string{"bind-key", "-T", "copy-mode-vi", "WheelDownPane", "send-keys", "-N3", "-X", "scroll-down"},
		)
		return cmds
	}
	// invisible: the frontend turns wheel events into Shift+PageUp/PageDown
	// key codes. In the root table S-PPage enters copy-mode (-e: leave it
	// again when scrolled back to the bottom); inside copy-mode both keys
	// scroll three lines. S-NPage outside copy-mode must not reach the shell
	// as a stray escape sequence, so it is bound to a harmless command.
	cmds = append(cmds,
		[]string{"set", "mouse", "off"},
		// The frontend writes several key codes in one burst per wheel event.
		// tmux's paste heuristic (assume-paste-time, default 1ms) would treat
		// such bursts as pasted text, skip the bindings and hand the raw
		// sequences to the shell ("...;2~;2~"). 0 disables the heuristic.
		[]string{"set", "assume-paste-time", "0"},
		[]string{"bind-key", "-n", "S-PPage", "copy-mode", "-e"},
		[]string{"bind-key", "-n", "S-NPage", "refresh-client"},
		// -N must precede -X: tmux treats everything after the -X command name
		// as that command's arguments, so "-X scroll-up -N 3" silently does nothing.
		[]string{"bind-key", "-T", "copy-mode", "S-PPage", "send-keys", "-N3", "-X", "scroll-up"},
		[]string{"bind-key", "-T", "copy-mode", "S-NPage", "send-keys", "-N3", "-X", "scroll-down"},
		[]string{"bind-key", "-T", "copy-mode-vi", "S-PPage", "send-keys", "-N3", "-X", "scroll-up"},
		[]string{"bind-key", "-T", "copy-mode-vi", "S-NPage", "send-keys", "-N3", "-X", "scroll-down"},
	)
	return cmds
}

// TmuxOptionArgs renders the option commands as argv tokens to append after
// the new-session arguments (tmux separates commands with a bare ";").
func TmuxOptionArgs(mode string) []string {
	var out []string
	for _, cmd := range TmuxOptionCommands(mode) {
		out = append(out, ";")
		out = append(out, cmd...)
	}
	return out
}

// TmuxOptionScript renders the option commands for a POSIX shell command
// line (" \; set ... \; bind-key ..."), quoting every token.
func TmuxOptionScript(mode string) string {
	var b strings.Builder
	for _, cmd := range TmuxOptionCommands(mode) {
		b.WriteString(` \; `)
		for i, tok := range cmd {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(posixQuote(tok))
		}
	}
	return b.String()
}

// posixQuote single-quotes a token unless it is plainly safe.
func posixQuote(tok string) string {
	safe := tok != ""
	for _, r := range tok {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=", r)) {
			safe = false
			break
		}
	}
	if safe {
		return tok
	}
	return "'" + strings.ReplaceAll(tok, "'", `'\''`) + "'"
}

// SetTmuxIntegrationMode selects how new terminals use tmux.
func (m *Manager) SetTmuxIntegrationMode(mode string) {
	m.ptyMutex.Lock()
	m.tmuxMode = NormalizeTmuxMode(mode)
	m.ptyMutex.Unlock()
}

// TmuxIntegrationMode returns the current tmux integration mode.
func (m *Manager) TmuxIntegrationMode() string {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	return NormalizeTmuxMode(m.tmuxMode)
}

// TmuxLiveUpdateCommands lists the commands that switch a running tmux
// session between the invisible and classic integrations: the session's
// mouse option plus the mode's key bindings (bindings are server-global; the
// other mode's bindings are harmless and left in place).
func TmuxLiveUpdateCommands(mode string, session string) [][]string {
	mode = NormalizeTmuxMode(mode)
	if mode == TmuxModeOff {
		return nil
	}
	var cmds [][]string
	for _, cmd := range TmuxOptionCommands(mode) {
		switch {
		case len(cmd) >= 3 && cmd[0] == "set" && (cmd[1] == "mouse" || cmd[1] == "assume-paste-time"):
			cmds = append(cmds, []string{"set", "-t", session + ":", cmd[1], cmd[2]})
		case cmd[0] == "bind-key":
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

// RenderTmuxArgs joins commands into argv tokens for one tmux invocation.
func RenderTmuxArgs(cmds [][]string) []string {
	var out []string
	for i, cmd := range cmds {
		if i > 0 {
			out = append(out, ";")
		}
		out = append(out, cmd...)
	}
	return out
}

// RenderTmuxScript joins commands into a POSIX shell string ("tmux a \; b").
func RenderTmuxScript(tmuxBin string, cmds [][]string) string {
	var b strings.Builder
	b.WriteString(tmuxBin)
	for i, cmd := range cmds {
		if i > 0 {
			b.WriteString(` \;`)
		}
		for _, tok := range cmd {
			b.WriteByte(' ')
			b.WriteString(posixQuote(tok))
		}
	}
	return b.String()
}

// SessionIDs returns the ids of all registered terminal sessions.
func (m *Manager) SessionIDs() []int {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	ids := make([]int, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// SetTmuxMouse records, after a live update, whether tmux owns the mouse in
// a terminal (local runtime metadata or SSH config).
func (m *Manager) SetTmuxMouse(id int, mouse bool) {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	if meta, ok := m.runtimeMeta[id]; ok {
		meta.TmuxMouse = mouse
		m.runtimeMeta[id] = meta
	}
	if meta, ok := m.sshMeta[id]; ok && meta != nil {
		meta.Config.TmuxMouse = mouse
	}
}
