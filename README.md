# Mimir

Mimir is an open source desktop terminal manager built with Go, Wails, Svelte, and xterm.js.

It focuses on local-first terminal workflows: local shells, SSH sessions, tmux-backed reconnects, command templates, notes, recordings, audit logs, and AI-assisted command workflows.

## Status

Mimir is pre-MVP software. Expect breaking changes while the terminal, SSH, recording, and update flows are hardened.

## Tested Platforms

Current manual runtime testing is limited to:

- Windows 11, version 10.0.26200, build 26200
- Debian 12, 64-bit, KDE desktop, running in VMware Workstation 17 Player

Other Windows, Linux, and macOS versions may build or run, but they are not yet validated by the maintainer. Treat non-listed platforms as experimental.

## Features

- Multi-session terminal UI with split panes
- Windows terminals via ConPTY
- Linux local terminals via PTY
- SSH profiles with password/key authentication
- SSH host-key verification
- tmux-first SSH reconnect support
- Optional per-profile SSH RC modes
- Local command templates
- Local notes panel
- Optional command history capture
- Terminal recording and scrubbed export
- AI-assisted command and workflow tools
- Local audit/activity logs
- Manual GitHub Releases update check

## Security Model

Mimir is local-first by default.

- Command history is opt-in. History is parsed from an OSC control channel that
  is treated as untrusted: fields are length-capped and control characters are
  rejected to prevent history poisoning from terminal output.
- SSH RC injection is opt-in per profile and does not write to remote `~/.bashrc`.
- AI context is sanitized and can be excluded from prompts.
- Credentials (SSH passwords, AI API key) are stored in the OS keyring when
  available. Without a keyring, they are kept in an encrypted file protected by
  a master password (Argon2id) plus a machine-bound secret, using envelope
  encryption. FIDO2/hardware-key unlock is supported as an additional key slot.
- Terminal recordings store raw output on disk. Keystroke (input) recording is
  off by default. Scrubbing is best-effort, pattern-based, and applied only on
  export — it cannot redact free-form secrets such as a typed password.
- `agg` GIF export downloads are pinned and SHA256-verified.
- App updates are checked manually through GitHub Releases. Auto-install is not enabled.

See [SECURITY.md](SECURITY.md) and [docs/security-notes.md](docs/security-notes.md).

## Requirements

- Go 1.25.x
- Node.js 22+
- npm
- Wails CLI v2

Install Wails:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Linux builds also need the Wails native dependencies. On Debian/Ubuntu:

```bash
sudo apt-get install libgtk-3-dev libwebkit2gtk-4.0-dev pkg-config build-essential
```

## Development

```bash
cd mimir
go mod tidy
cd frontend
npm install
cd ..
wails dev
```

Run tests:

```bash
go test ./...
cd frontend
npm run build
```

## Release Builds

Local Windows + Linux release from PowerShell:

```powershell
.\scripts\release-all.ps1 -Version 0.1.0 -UpdateRepository OWNER/REPO
```

Linux:

```bash
./scripts/release-linux.sh 0.1.0 --update-repo=OWNER/REPO
```

macOS:

```bash
./scripts/release-macos.sh 0.1.0 --update-repo=OWNER/REPO
```

Artifacts and `checksums.txt` are written to `build/release/`.

See [docs/releasing.md](docs/releasing.md).

## GitHub Releases

The repository includes GitHub Actions for:

- CI on `main` and pull requests
- Cross-platform release builds on tags matching `v*`
- Publishing release artifacts and checksums

To publish a release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Project Layout

```text
.
├── app*.go                 # Wails backend bindings
├── terminal/               # Local and SSH terminal management
├── ssh/                    # SSH profiles, keys, known hosts, secrets
├── template/               # Command templates
├── workflow/               # Workflow engine
├── aiflow/                 # AI tool-flow policy and context handling
├── history/                # Optional command history
├── recording/              # Asciicast recording and GIF export
├── frontend/               # Svelte frontend
├── docs/                   # Architecture, security, testing, release docs
└── scripts/                # Local release scripts
```

## License

MIT. See [LICENSE](LICENSE).
