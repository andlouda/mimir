# Development

## Requirements

- Go 1.25.x. The module declares `go 1.25.0` and `toolchain go1.25.10`.
- Node.js 22+
- npm
- Wails CLI v2

Install Wails:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Linux/WSL native dependencies for Wails:

```bash
sudo apt-get update
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev pkg-config build-essential
```

## Setup

```bash
cd mimir
go mod tidy
cd frontend
npm install
cd ..
```

On WSL, avoid sharing `frontend/node_modules` between Windows and Linux. If native Vite/Rolldown bindings break, rebuild dependencies inside the environment you are using:

```bash
cd frontend
rm -rf node_modules package-lock.json
npm install
```

PowerShell equivalent:

```powershell
Remove-Item -Recurse -Force .\frontend\node_modules -ErrorAction SilentlyContinue
Remove-Item -Force .\frontend\package-lock.json -ErrorAction SilentlyContinue
Set-Location .\frontend
npm install
Set-Location ..
```

## Development Server

```bash
wails dev
```

Wails starts the Vite dev server through `frontend:dev:serverUrl: "auto"` in `wails.json`.

## Common Checks

```bash
go test ./...
cd frontend
npm run build
```

For reproducible Go cache locations in WSL:

```bash
GOMODCACHE=/tmp/mimir-go-mod-cache GOCACHE=/tmp/mimir-go-build-cache GOTOOLCHAIN=auto go test ./...
```

## Release Builds

Windows and Linux from Windows PowerShell:

```powershell
.\scripts\release-all.ps1 -Version dev
```

With embedded update metadata:

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

Artifacts and checksums are written to `build/release/`.

## Wails Bindings

When public methods on `App` change, Wails regenerates bindings during `wails dev` or `wails build`.

Generated files live in:

```text
frontend/wailsjs/
```

Do not edit them manually.

## Code Organization

The backend uses feature-specific `app_*.go` files for Wails bindings and small packages for storage/runtime code.

The frontend still has a large `App.svelte`. New feature-heavy UI should prefer smaller components under `frontend/src/lib/`.

## CI

GitHub Actions are included:

- `.github/workflows/ci.yml`: Go tests and frontend build
- `.github/workflows/release.yml`: cross-platform release artifacts on `v*` tags

The release workflow publishes artifacts and `checksums.txt` to GitHub Releases.
