# Testing

## Current Test Command

```bash
go test ./...
```

This is expected to pass on Linux/WSL, Windows, and macOS because terminal implementations are split by build tags:

- Windows: ConPTY implementation
- non-Windows: PTY implementation

Frontend production build check:

```bash
cd frontend
npm run build
```

## Package Coverage

The project uses Go's standard `testing` package. There is no separate assertion library.

Covered areas include:

- session persistence
- template loading/application
- strict JSON parsing
- SSH/tmux helper behavior
- AI prompt/context sanitization
- AI tool discovery and guardrails
- workflow engine/playbooks
- transcript storage
- command history parser/store
- terminal helper behavior
- update version comparison and repository validation

Packages without dedicated tests yet:

- `recording`
- `notes`
- `folder`
- some Wails binding facade methods
- most frontend UI behavior

## Frontend Tests

There is currently no Vitest/Playwright test setup.

The current frontend gate is:

```bash
npm run build
```

Known build warnings:

- Svelte a11y warnings for a few interactive `div`s
- Vite/Rolldown chunk-size warning

These warnings do not currently fail the build.

## Recommended MVP Gates

Before sharing a release artifact:

- `go test ./...`
- `npm run build` in `frontend`
- local smoke test for at least one local terminal
- SSH profile smoke test with tmux installed
- SSH profile smoke test without tmux
- host-key unknown/mismatch behavior checked
- command history toggle checked
- recording start/stop/export checked if touched
- update check card checked in Settings

## Useful Commands

Go tests with explicit WSL caches:

```bash
GOMODCACHE=/tmp/mimir-go-mod-cache GOCACHE=/tmp/mimir-go-build-cache GOTOOLCHAIN=auto go test ./...
```

Frontend dependency reinstall after Windows/WSL mixing:

```bash
cd frontend
rm -rf node_modules package-lock.json
npm install
npm run build
```

Release-script syntax checks:

```bash
bash -n scripts/release-linux.sh
bash -n scripts/release-macos.sh
```

PowerShell scripts should be syntax-checked on Windows or in an environment with `pwsh` installed.
