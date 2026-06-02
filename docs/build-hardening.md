# Build Hardening

## Current Baseline

The project now includes:

- local release scripts for Windows, Linux, and macOS
- GitHub CI for tests/build
- GitHub Release workflow for tag-based artifacts
- `checksums.txt` generation
- build-time `AppVersion` and `UpdateRepository`
- manual in-app update check

## Required Local Gate

Before sharing a build:

```bash
go test ./...
cd frontend
npm run build
```

For WSL:

```bash
GOMODCACHE=/tmp/mimir-go-mod-cache GOCACHE=/tmp/mimir-go-build-cache GOTOOLCHAIN=auto go test ./...
```

## Release Gate

Use the release scripts as the packaging source of truth:

```powershell
.\scripts\release-all.ps1 -Version 0.1.0 -UpdateRepository OWNER/REPO
```

or per platform:

```bash
./scripts/release-linux.sh 0.1.0 --update-repo=OWNER/REPO
./scripts/release-macos.sh 0.1.0 --update-repo=OWNER/REPO
```

## GitHub Actions

CI:

```text
.github/workflows/ci.yml
```

Release:

```text
.github/workflows/release.yml
```

The release workflow runs on tags matching `v*` and uploads artifacts plus `checksums.txt`.

## Dependency Notes

Linux build dependencies for Wails:

```bash
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev pkg-config build-essential
```

Node modules are platform-specific. Do not reuse a Windows `node_modules` directory in WSL.

## Future Hardening

Recommended next steps:

- sign Windows artifacts
- sign/notarize macOS artifacts
- add SBOM generation
- add `govulncheck` to CI
- add frontend tests
- fail CI on Svelte a11y warnings once current warnings are cleaned up
- add a signed updater only after artifact signing is in place
