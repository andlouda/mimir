//go:build windows
// +build windows

package terminal

import (
	"encoding/base64"
	"fmt"
	"log"
	"mimir/executil"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/UserExistsError/conpty"
)

const conptyBashRcBase = "source ~/.bashrc 2>/dev/null || true\n"

// conptyBashHistoryHook emits OSC 7337 on every prompt. A new command carries
// its command line; an unchanged prompt still reports the current cwd (an
// empty-command beacon) so cwd-dependent features work before any command runs.
const conptyBashHistoryHook = `__mimir_last_cmd=""
__mimir_precmd() {
  local exit_code=$?
  local cmd; cmd=$(HISTTIMEFORMAT= history 1 2>/dev/null | sed 's/^ *[0-9]* *//')
  local b64=""
  if [ -n "$cmd" ] && [ "$cmd" != "$__mimir_last_cmd" ]; then
    __mimir_last_cmd="$cmd"
    b64=$(printf '%s' "$cmd" | base64 2>/dev/null | tr -d '\n')
  fi
  printf '\033]7337;cmd=%s;exit=%s;cwd=%s;host=%s;user=%s;shell=bash;ts=%s\007' \
    "$b64" "$exit_code" "$PWD" "$(hostname -s 2>/dev/null || echo unknown)" "$(whoami)" "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"
}
PROMPT_COMMAND="__mimir_precmd;${PROMPT_COMMAND}"
`

// conptyPowerShellHistoryHook emits OSC 7337 on every prompt. A new command
// carries its command line; an unchanged prompt still reports the current cwd
// (an empty-command beacon) so cwd-dependent features work before any command
// has been run.
const conptyPowerShellHistoryHook = `$global:__mimir_last_cmd = ''
$global:__mimir_prev_prompt = $function:global:prompt
function global:prompt {
  $success = $?
  $nativeExit = $global:LASTEXITCODE
  $cmd = ''
  $historyItem = Get-History -Count 1 -ErrorAction SilentlyContinue
  if ($historyItem -and $historyItem.CommandLine) {
    $candidate = [string]$historyItem.CommandLine
    if ($candidate -and $candidate -ne $global:__mimir_last_cmd) {
      $global:__mimir_last_cmd = $candidate
      $cmd = $candidate
    }
  }
  $exitCode = if ($null -ne $nativeExit) { [string]$nativeExit } elseif ($success) { '0' } else { '1' }
  $b64 = if ($cmd) { [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($cmd)) } else { '' }
  $cwd = (Get-Location).Path
  $hostName = $env:COMPUTERNAME
  if (-not $hostName) { $hostName = 'unknown' }
  $userName = $env:USERNAME
  if (-not $userName) { $userName = 'unknown' }
  $ts = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
  [Console]::Write("$([char]27)]7337;cmd=$b64;exit=$exitCode;cwd=$cwd;host=$hostName;user=$userName;shell=powershell;ts=$ts$([char]7)")
  return __MIMIR_PROMPT__
}
`

// conptyPowerShellPromptOnly keeps Mimir's short prompt when the beacon
// profile is not installed (history off, no consent).
const conptyPowerShellPromptOnly = `function global:prompt { "$env:USERNAME ❯ " }
`

// powerShellProfileContent renders the profile for the current prompt mode:
// the beacon prompt returns Mimir's short prompt or PowerShell's default
// text, so the beacon is never replaced by a prompt typed from the frontend.
func powerShellProfileContent(withBeacon bool) string {
	mimir := PromptMode() == PromptModeMimir
	if !withBeacon {
		if mimir {
			return conptyPowerShellPromptOnly
		}
		return ""
	}
	// Shell mode: the beacon runs, then whatever prompt the user's own
	// $PROFILE defined (oh-my-posh, Starship, ...) draws the line.
	text := `(& $global:__mimir_prev_prompt)`
	if mimir {
		text = `"$env:USERNAME ❯ "`
	}
	return strings.Replace(conptyPowerShellHistoryHook, "__MIMIR_PROMPT__", text, 1)
}

var tmuxNamePattern = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// sanitizeTmuxName strips characters that are unsafe in a tmux session name
// (and in the shell command line it is interpolated into).
func sanitizeTmuxName(name string) string {
	name = tmuxNamePattern.ReplaceAllString(name, "")
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

func conptyBashRcBase64(wsl bool) string {
	rcContent := conptyBashRcBase
	// Same rule as the unix path: Mimir's short prompt only in "mimir" mode.
	if PromptMode() == PromptModeMimir {
		rcContent += "PS1='\\W \\$ '\n"
	}
	if isHistoryEnabled() || shellHookConsent() {
		rcContent += conptyBashHistoryHook
	}
	// Extra env (agent telemetry) travels inside the rc file: for WSL that
	// is the only way across the Windows/Linux boundary.
	rcContent += launchEnvExports(wsl)
	return base64.StdEncoding.EncodeToString([]byte(rcContent))
}

func conptyPowerShellCommand() string {
	content := powerShellProfileContent(isHistoryEnabled() || shellHookConsent())
	if content == "" {
		return "powershell.exe"
	}
	profilePath, err := writeMimirShellFile("mimir-profile.ps1", content)
	if err != nil {
		log.Printf("history: failed to write PowerShell hook profile: %v", err)
		return "powershell.exe"
	}
	// Plain double quotes: strconv.Quote would double every backslash.
	return fmt.Sprintf("powershell.exe -NoLogo -NoExit -ExecutionPolicy Bypass -File \"%s\"", profilePath)
}

// conptySession wraps a Windows ConPTY and implements TerminalSession.
type conptySession struct {
	cpty   *conpty.ConPty
	closed bool
	mu     sync.Mutex
}

func (s *conptySession) Read(p []byte) (int, error) {
	return s.cpty.Read(p)
}

func (s *conptySession) Write(p []byte) (int, error) {
	return s.cpty.Write(p)
}

func (s *conptySession) Resize(rows, cols uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	return s.cpty.Resize(int(cols), int(rows))
}

func (s *conptySession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.cpty.Close()
}

// StartTerminalWithOptions starts a new terminal session. Windows ignores local tmux options.
func (m *Manager) StartTerminalWithOptions(terminalType string, tmuxSessionName string) (int, error) {
	tmuxMode := m.TmuxIntegrationMode()
	if tmuxMode == TmuxModeOff {
		tmuxSessionName = ""
	}
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	var (
		cmdString string
		usingTmux bool
		tmuxName  string
	)
	switch terminalType {
	case "cmd":
		if runtime.GOOS == "windows" {
			cmdString = "cmd.exe"
		} else {
			return 0, fmt.Errorf("cmd.exe is only available on Windows")
		}
	case "powershell":
		if runtime.GOOS == "windows" {
			cmdString = conptyPowerShellCommand()
		} else {
			return 0, fmt.Errorf("powershell.exe is only available on Windows")
		}
	case "wsl":
		if runtime.GOOS == "windows" {
			inner := `mkdir -p ~/.cache/mimir/shell; printf ` + conptyBashRcBase64(true) + ` | base64 -d > ~/.cache/mimir/shell/bashrc; `
			if name := sanitizeTmuxName(tmuxSessionName); name != "" {
				// Run the shell inside a tmux session (attach-or-create) so the
				// session — and anything running in it, e.g. claude — survives
				// closing and reopening the app. Falls back to a plain shell
				// when tmux is not installed in the WSL distro.
				// set-clipboard on + Ms override: copy tmux mouse selections and
				// OSC 52 writes from the pane (nested tmux, vim) to the clipboard.
				// The unbinds drop tmux's right-click menu
				// (Mimir draws its own); the wheel bindings soften scroll steps.
				inner += `if command -v tmux >/dev/null 2>&1; then exec tmux -L ` + executil.TmuxSocket + ` new-session -A -s ` + name +
					` 'bash --rcfile ~/.cache/mimir/shell/bashrc -i'` + TmuxOptionScript(tmuxMode) + `; ` +
					`else exec bash --rcfile ~/.cache/mimir/shell/bashrc -i; fi`
				usingTmux = true
				tmuxName = name
			} else {
				inner += `exec bash --rcfile ~/.cache/mimir/shell/bashrc -i`
			}
			cmdString = `wsl.exe -- bash -lc "` + inner + `"`
		} else {
			cmdString = "bash"
		}
	case "bash":
		cmdString = `bash -lc "mkdir -p ~/.cache/mimir/shell; printf ` + conptyBashRcBase64(false) + ` | base64 -d > ~/.cache/mimir/shell/bashrc; exec bash --rcfile ~/.cache/mimir/shell/bashrc -i"`
	case "zsh":
		cmdString = `zsh -lc "export PROMPT='%1~ %# '; exec zsh -f -i"`
	default:
		return 0, fmt.Errorf("unsupported terminal type: %s", terminalType)
	}

	var opts []conpty.ConPtyOption
	if extra := launchEnv(); len(extra) > 0 {
		opts = append(opts, conpty.ConPtyEnv(append(os.Environ(), extra...)))
	}
	c, err := conpty.Start(cmdString, opts...)
	if err != nil {
		return 0, fmt.Errorf("failed to start pty for %s: %w", terminalType, err)
	}

	m.ptyLastID++
	id := m.ptyLastID
	m.sessions[id] = &conptySession{cpty: c}
	m.readySignals[id] = make(chan struct{})

	meta := TerminalRuntimeMeta{}
	if usingTmux {
		meta.TmuxActive = true
		meta.TmuxSessionName = tmuxName
		meta.TmuxMouse = TmuxMouseEnabled(tmuxMode)
		meta.TmuxMode = "auto"
		meta.TmuxStatus = "active"
	}
	m.runtimeMeta[id] = meta

	return id, nil
}

// Pid is the ConPTY child at the root of the pane's process tree.
func (s *conptySession) Pid() int {
	if s.cpty != nil {
		return s.cpty.Pid()
	}
	return 0
}
