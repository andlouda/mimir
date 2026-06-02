# ADR-0002: Plattform-PTY fuer lokale Terminals

## Status

Angenommen, aktualisiert

## Kontext

Mimir muss echte interaktive Terminal-Sitzungen verwalten. xterm.js im Frontend braucht dafuer einen Backend-PTY, der ANSI/VT-Sequenzen, interaktive Programme und Resize korrekt unterstuetzt.

Windows und Unix-artige Systeme brauchen unterschiedliche Implementierungen.

## Entscheidung

Mimir verwendet plattformspezifische Terminal-Backends hinter einem gemeinsamen `TerminalSession`-Interface.

Windows:

- `terminal/terminal.go`
- `github.com/UserExistsError/conpty`
- Shells: `cmd`, `powershell`, `wsl`, `bash`, `zsh`

Linux/macOS:

- `terminal/terminal_unix.go`
- `github.com/creack/pty`
- Shells: `bash`, `zsh`, `sh` fallback where applicable

SSH:

- `terminal/ssh_session.go`
- `golang.org/x/crypto/ssh`
- same `TerminalSession` interface

## Consequences

### Positiv

- Gemeinsame Manager-Logik fuer lokale und SSH-Terminals
- Linux/macOS sind nicht mehr Stub-only
- Tests koennen auf Nicht-Windows-Systemen laufen
- SSH-Sessions koennen wie lokale Terminals geschrieben, resized und geschlossen werden
- Recording und History-Parsing koennen zentral in der Read-Loop passieren

### Negativ

- Unterschiedliche Shell-Startlogik pro Plattform
- Windows/WSL und Linux haben unterschiedliche rcfile-Mechaniken
- Native GUI/Wails-Abhaengigkeiten bleiben plattformspezifisch

## Notes

Command history hooks are optional and consent-gated. The backend also checks consent before storing parsed OSC-7337 commands.

SSH tmux handling is orchestrated in `app_ssh.go` and represented as terminal metadata.

## Betroffene Dateien

- `terminal/terminal.go`
- `terminal/terminal_unix.go`
- `terminal/ssh_session.go`
- `app_ssh.go`
- `app_history.go`
- `recording/`
