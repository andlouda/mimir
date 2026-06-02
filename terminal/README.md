# terminal

## Purpose

The `terminal` package manages local and SSH terminal sessions behind a shared interface. It starts processes, reads terminal output, emits Wails events, handles resize/write/close, tracks tmux metadata, records sessions, and parses command-history OSC sequences.

## Files

| File | Purpose |
|------|---------|
| `terminal.go` | Windows implementation using ConPTY. |
| `terminal_unix.go` | Linux/macOS implementation using `github.com/creack/pty`. |
| `ssh_session.go` | SSH PTY session wrapper using `golang.org/x/crypto/ssh`. |
| `session.go` | Shared session interface/types. |
| `terminal_unix_test.go` | Non-Windows helper tests. |

## Responsibilities

- Start local shells: `cmd`, `powershell`, `wsl`, `bash`, `zsh`
- Register SSH sessions created by the app layer
- Stream output through Wails events
- Receive frontend input and write to the session
- Resize PTYs
- Handle disconnect/close events
- Track tmux status metadata
- Start/stop asciicast recordings
- Parse and strip OSC-7337 command-history events

## Terminal Session Interface

```go
type TerminalSession interface {
    Read([]byte) (int, error)
    Write([]byte) (int, error)
    Resize(rows, cols uint16) error
    Close() error
}
```

Local and SSH sessions implement the same interface.

## Events

| Event | Meaning |
|-------|---------|
| `terminal-output-{id}` | Terminal output chunk. |
| `terminal-closed-{id}` | Session exited/closed. |
| `terminal-disconnected-{id}` | SSH connection disconnected. |

## Startup Sequence

```text
StartTerminal / RegisterSSHSession
Frontend creates xterm
Frontend subscribes to events
ConfirmFrontendReady(id)
InitializeTerminal(id)
Backend starts read loop
```

`ConfirmFrontendReady` prevents early output from being emitted before the frontend subscribed to events.

## Command History

History capture uses OSC-7337 sequences. The terminal manager strips the sequence from output before sending data to the frontend.

Insertion into the history store is guarded by the history consent check. Parsed commands are ignored when tracking is disabled.

## Recording

The manager can attach a recording writer to a terminal session. It records output/input/resize events as asciicast v2 frames.

## Platform Notes

Windows:

- Uses ConPTY.
- `wsl`/`bash` can use generated rcfiles for short prompt and optional history hook.

Linux/macOS:

- Uses `creack/pty`.
- bash/zsh wrappers source the user's normal rcfile, set a compact prompt, and optionally add the history hook when consent is enabled.

SSH:

- Created in `app_ssh.go`, registered here.
- tmux status is stored in `SSHMeta.Config`.
