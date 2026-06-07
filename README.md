# Mimir

> [!CAUTION]
> This project is almost entirely vibe-coded. Use at your own risk.

Mimir is an open source desktop terminal manager built with Go, Wails, Svelte, and xterm.js.

It focuses on local-first terminal workflows: local shells, SSH sessions, tmux-backed reconnects, command templates, notes, recordings, audit logs, and AI-assisted command workflows.

## Status

Mimir is pre-MVP software. Expect breaking changes while the terminal, SSH, recording, and update flows are hardened.

## Platform Support

| Platform | Status | Notes |
|----------|--------|-------|
| Windows 11 | **Primary** | Main development platform, most tested |
| Linux (Debian/Ubuntu) | Experimental | Builds and runs; WebKit2GTK quirks possible |
| macOS (Apple Silicon) | Experimental | CI-built; limited manual testing |

Other OS versions may build or run but are not validated. Treat unlisted platforms as unsupported.

## Features

- Multi-session terminal UI with split panes and folders
- Windows terminals via ConPTY, Linux via PTY
- SSH profiles with password/key authentication and jump hosts
- SSH host-key verification (TOFU)
- tmux-backed SSH reconnect with unique session isolation
- SFTP file browser for remote hosts
- Terminal scrollback search (Ctrl+Shift+F)
- 58 built-in command templates with discovery-driven variables
- Workflow engine with playbooks, approval flow, and AI steps
- Workflow picker (Ctrl+Shift+W) for quick playbook execution
- AI integration (OpenAI, Ollama, Anthropic) with guardrails
- Local notes panel with markdown support
- Optional command history capture and search
- Terminal recording with scrubbed export and GIF generation
- Self-update with SHA256 verification and staged install
- Local audit/activity logs
- i18n (English, German)

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
- Self-updates download from GitHub Releases, verify SHA256 checksums, stage
  the binary, and apply on restart. The running binary never modifies itself.

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

The entire buildable application lives in `app/`. Run all build and test
commands from there.

```bash
cd mimir/app
go mod tidy
cd frontend
npm install
cd ..
wails dev
```

Run tests:

```bash
cd app
go test ./...
cd frontend
npm run build
```

## Release Builds

Local Windows + Linux release from PowerShell:

```powershell
.\app\scripts\release-all.ps1 -Version 0.1.0 -UpdateRepository OWNER/REPO
```

Linux:

```bash
./app/scripts/release-linux.sh 0.1.0 --update-repo=OWNER/REPO
```

macOS:

```bash
./app/scripts/release-macos.sh 0.1.0 --update-repo=OWNER/REPO
```

Artifacts and `checksums.txt` are written to `app/build/release/`.

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
├── app/                    # The buildable application (Go + Svelte + Wails)
│   ├── *.go                # Wails backend (package main: App struct + bindings)
│   ├── terminal/           # Local and SSH terminal management
│   ├── ssh/                # SSH profiles, keys, known hosts, secrets
│   ├── template/           # Command templates
│   ├── workflow/           # Workflow engine, playbooks, approval
│   ├── update/             # Update checker, downloader, staging
│   ├── aiflow/             # AI tool-flow policy and context handling
│   ├── history/            # Optional command history
│   ├── recording/          # Asciicast recording and GIF export
│   ├── notes/              # Markdown notes storage
│   ├── folder/             # Terminal folder grouping
│   ├── safeio/             # Atomic file writes
│   ├── frontend/           # Svelte frontend
│   ├── templates/          # Built-in command templates (Wails assetdir)
│   ├── build/              # Wails build assets (icon, platform manifests)
│   ├── scripts/            # Local release scripts
│   ├── go.mod  wails.json  # Go module + Wails config
├── docs/                   # Architecture, security, testing, release docs
├── .github/                # CI and release workflows
└── README.md  LICENSE  SECURITY.md  CONTRIBUTING.md
```

> **Why is `package main` at `app/` root and not split into subpackages?**
> Wails v2 generates its JS bindings from the `App` struct in `package main`
> and expects it alongside `wails.json`. The backend files (`app_*.go`, `ai*.go`,
> …) are all methods on `*App`, so Go requires them in one directory.

## License

MIT. See [LICENSE](LICENSE).
