# Local Release

Use these scripts to create shareable desktop artifacts before or alongside GitHub Actions releases.

## Windows + Linux From PowerShell

```powershell
cd path/to/mimir
.\scripts\release-all.ps1 -Version dev
```

With embedded GitHub update metadata:

```powershell
.\scripts\release-all.ps1 -Version 0.1.0 -UpdateRepository OWNER/REPO
```

Fast local iteration:

```powershell
.\scripts\release-all.ps1 -Version dev -SkipChecks -SkipAudit
```

Artifacts:

```text
build\release\mimir-windows-amd64-VERSION.zip
build\release\mimir-linux-amd64-VERSION.tar.gz
build\release\checksums.txt
```

## Windows Only

```powershell
.\scripts\release-local.ps1 -Version 0.1.0 -UpdateRepository OWNER/REPO
```

Artifact:

```text
build\release\mimir-windows-amd64-0.1.0.zip
```

## Linux

```bash
chmod +x scripts/release-linux.sh
./scripts/release-linux.sh 0.1.0 --update-repo=OWNER/REPO
```

Artifact:

```text
build/release/mimir-linux-amd64-0.1.0.tar.gz
```

## macOS

macOS builds must run on macOS:

```bash
chmod +x scripts/release-macos.sh
./scripts/release-macos.sh 0.1.0 --update-repo=OWNER/REPO
```

Artifact:

```text
build/release/mimir-darwin-universal-0.1.0.zip
```

## What The Scripts Do

- Go tests
- Go race tests on Windows/Linux release scripts
- frontend install/build
- optional npm audit
- optional govulncheck
- Wails build with `AppVersion` and optional `UpdateRepository`
- package artifact
- update `checksums.txt`

## Smoke Test

Before sharing:

- start app
- open local terminal
- create split
- close split
- restart app and check restored transcript behavior
- create SSH session with tmux installed
- create SSH session without tmux and verify `no tx`
- test host-key unknown/mismatch path if possible
- run one low-risk template
- start/stop recording
- open Settings and run update check
