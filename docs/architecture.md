# Architecture

## Overview

Mimir is a Wails v2 desktop app with a Go backend and a Svelte 5 frontend.

The app is local-first and focuses on terminal workflows:

- local Windows terminals through ConPTY
- local Linux/macOS terminals through PTY
- SSH terminals with host-key verification
- tmux-first SSH reconnects
- command templates
- optional command history
- terminal recordings
- notes
- AI-assisted command and workflow tooling
- local audit/activity logs
- manual GitHub Releases update checks

## Runtime Shape

```text
Wails desktop app
├── Go backend
│   ├── terminal manager
│   ├── SSH profile / secret / known-host stores
│   ├── templates
│   ├── sessions and transcripts
│   ├── command history
│   ├── recordings
│   ├── AI/tool-flow runtime
│   ├── audit/activity logs
│   └── update checker
└── Svelte frontend
    ├── xterm.js terminal panes
    ├── sidebar navigation
    ├── SSH profile UI
    ├── template manager
    ├── notes
    ├── recording player
    ├── history dashboard
    ├── AI/workflow views
    └── settings
```

## Backend Packages

| Package | Responsibility |
|---------|----------------|
| `main` | Wails app setup and binding facade. Files are split by feature: `app_ssh.go`, `app_history.go`, `app_recording.go`, `app_update.go`, etc. |
| `terminal` | Local PTY/ConPTY sessions, SSH session wrapping, output streaming, resize/write/close, tmux metadata, recording hooks, command-history OSC parsing. |
| `ssh` | SSH profiles, key discovery, known-hosts, and secret storage. |
| `session` | Persisted terminal state in the user config directory. |
| `transcript` | Terminal transcript persistence and restore excerpts. |
| `template` | Embedded and editable command templates. |
| `history` | Optional command history database and OSC-7337 parser. |
| `recording` | Asciicast recording, scrubbing, trimming, GIF export, pinned `agg` download. |
| `notes` | Local Markdown-style notes storage. |
| `folder` | Custom sidebar terminal folders. |
| `activitylog` | Local activity/audit events. |
| `aiflow`, `tools`, `workflow` | AI prompt context, tool registry, workflow/playbook execution. |
| `update` | GitHub Releases update check and platform asset detection. |
| `safeio` | Atomic file writes. |

## Terminal Architecture

The terminal manager exposes one common session interface for local and SSH terminals:

```go
type TerminalSession interface {
    Read([]byte) (int, error)
    Write([]byte) (int, error)
    Resize(rows, cols uint16) error
    Close() error
}
```

Platform implementations:

- `terminal/terminal.go`: Windows build, ConPTY via `github.com/UserExistsError/conpty`
- `terminal/terminal_unix.go`: non-Windows build, PTY via `github.com/creack/pty`
- `terminal/ssh_session.go`: SSH PTY session via `golang.org/x/crypto/ssh`

Terminal output is read in a backend goroutine and emitted to the frontend through Wails events:

```text
terminal-output-{id}
terminal-closed-{id}
terminal-disconnected-{id}
```

The frontend must call `ConfirmFrontendReady(id)` before `InitializeTerminal(id)` starts the reader. This avoids dropping initial output before xterm is subscribed.

## SSH And tmux

SSH profiles are stored locally in `ssh_profiles.json`. Passwords and key passphrases are stored in the system keyring when available, otherwise in an encrypted local fallback store.

By default, SSH profiles use tmux when available. Mimir probes the host for tmux and exposes terminal metadata:

- `tx`: tmux active
- `no tx`: tmux missing
- `clean`: no Mimir RC injection
- `rc`: per-profile RC mode enabled

tmux session names are deterministic per SSH profile, so reconnecting lands in the same session when the host supports tmux.

Remote RC handling is opt-in per profile:

- `off`: no Mimir RC behavior
- `remote-default`: start the remote shell as interactive
- `mimir`: start a minimal clean shell
- `local-snippet`: upload a local snippet to `~/.cache/mimir/shell/...` and start bash with that rcfile

Mimir does not write to remote `~/.bashrc`.

## History Capture

Command history is opt-in. The app uses an OSC-7337 escape sequence emitted by shell hooks. The backend parser strips this sequence from terminal output before it reaches xterm.

The backend enforces consent at insert time. Even if a shell emits OSC-7337, commands are not stored unless history tracking is enabled.

History data is stored locally in SQLite under the user config directory.

## Recording

Terminal sessions can be recorded as asciicast v2 files. Recording hooks capture:

- terminal output
- terminal input
- resize events

Exports support raw, scrubbed, trimmed, and GIF output. GIF export uses `agg`. The app's built-in download path is pinned to a known `agg` version and verifies SHA256, size, and timeout before installing the binary into the local Mimir config bin directory.

## AI And Workflow

AI settings are local. The app supports Ollama and OpenAI-compatible Responses API settings.

AI context handling includes:

- optional terminal output inclusion
- provider-aware sanitization
- granular sanitization audit metadata
- workflow modes: `assist`, `approve`, `auto`

`auto` is intentionally marked experimental in the UI. Backend guardrails and approval policy still control execution.

## Update Checks

Mimir has a manual GitHub Releases update check.

Release builds can embed:

```text
main.AppVersion
main.UpdateRepository
```

The checker reads:

```text
https://api.github.com/repos/OWNER/REPO/releases/latest
```

It compares semantic versions, detects platform assets, and surfaces checksum assets. It does not auto-install updates.

## Frontend

The frontend is Svelte 5 with Vite 8 and xterm.js 6.

Main files:

- `frontend/src/App.svelte`: app shell, sidebar, terminal orchestration, settings, modals
- `frontend/src/lib/SplitPane.svelte`: split-pane terminal layout
- `frontend/src/lib/FileBrowser.svelte`: local/remote file browser
- `frontend/src/lib/TemplateManager.svelte`: templates
- `frontend/src/lib/MarkdownNotes.svelte`: notes
- `frontend/src/lib/HistoryDashboard.svelte`: command history
- `frontend/src/lib/RecordingPlayer.svelte`: recordings
- `frontend/src/lib/workflows/WorkflowBuilder.svelte`: workflows
- `frontend/src/lib/terminals/*.js`: xterm/tmux/layout helpers

`frontend/wailsjs/` is generated by Wails and should not be edited manually.

## Persistence

Typical user-data locations:

| Data | Location |
|------|----------|
| session state | `<UserConfigDir>/mimir/mimir_session.json` |
| terminal transcripts | `<UserConfigDir>/mimir/transcripts/` |
| SSH profiles | `<UserConfigDir>/mimir/ssh_profiles.json` |
| known hosts | `<UserConfigDir>/mimir/known_hosts` |
| secret fallback | `<UserConfigDir>/mimir/ssh_secrets.enc` |
| command history | `<UserConfigDir>/mimir/command_history.db` |
| recordings | `<UserConfigDir>/mimir/recordings/` |
| notes | `<UserConfigDir>/mimir/notes/` |
| activity logs | `<UserConfigDir>/mimir/activity.log` |
| local helper binaries | `<UserConfigDir>/mimir/bin/` |

Files containing user state or secrets should not be committed.
