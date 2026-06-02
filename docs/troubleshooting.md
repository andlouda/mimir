# Troubleshooting

## `wails dev` fails with `vite` not found

Run frontend install in the same environment where you start Wails:

```bash
cd frontend
npm install
cd ..
wails dev
```

PowerShell:

```powershell
Set-Location .\frontend
npm install
Set-Location ..
wails dev
```

## WSL/Linux frontend build misses native binding

Symptom examples:

- missing `@rolldown/binding-linux-x64-gnu`
- `vite` exists on Windows but not WSL

Cause: `node_modules` was created in another OS environment.

Fix:

```bash
cd frontend
rm -rf node_modules package-lock.json
npm install
npm run build
```

PowerShell equivalent:

```powershell
Remove-Item -Recurse -Force .\frontend\node_modules -ErrorAction SilentlyContinue
Remove-Item -Force .\frontend\package-lock.json -ErrorAction SilentlyContinue
Set-Location .\frontend
npm install
npm run build
Set-Location ..
```

## PowerShell `rm -rf` fails

PowerShell does not support Unix `rm -rf`.

Use:

```powershell
Remove-Item -Recurse -Force .\frontend\node_modules
Remove-Item -Force .\frontend\package-lock.json
```

## `go test ./...` opens `frontend/node_modules/fsevents`

Cause: Go package discovery can walk into mixed/generated frontend dependencies in some environments.

Fix: remove/reinstall frontend dependencies in the current environment. The normal current command should pass:

```bash
GOMODCACHE=/tmp/mimir-go-mod-cache GOCACHE=/tmp/mimir-go-build-cache GOTOOLCHAIN=auto go test ./...
```

If it still walks `frontend/node_modules`, run Go tests from the repo root after deleting `frontend/node_modules`.

## Linux app starts but local terminal behavior differs

Linux/macOS terminals now use native PTY, not a stub.

Expected behavior:

- bash/zsh are resolved from PATH and common system paths
- shell rc wrappers set a compact prompt
- history hook is only added when command history is enabled
- local tmux can be used where supported by the UI/backend

If a shell fails to start, check:

```bash
command -v bash
command -v zsh
```

## SSH connects but no tmux badge

Mimir shows:

- `tx`: tmux active
- `no tx`: tmux missing
- no badge/disabled: profile has tmux disabled

On Debian/Ubuntu remote hosts:

```bash
sudo apt install tmux
```

Other common installs:

```bash
sudo dnf install tmux
sudo pacman -S tmux
sudo apk add tmux
```

Reconnect only preserves shell state when the remote host has tmux and the profile uses tmux.

## SSH RC mode does not appear to apply

RC mode applies when Mimir starts a new shell. If tmux reconnects to an existing session, that existing shell keeps its previous environment.

Use a new tmux session or kill the old session when testing RC behavior.

Mimir does not edit remote `~/.bashrc`; uploaded local snippets go under:

```text
~/.cache/mimir/shell/
```

## Host-key verification dialog appears

This is expected for unknown or changed SSH host keys.

- Unknown key: verify fingerprint, then accept.
- Mismatch: treat as suspicious unless you intentionally rebuilt/rekeyed the host.

## Command history is not recorded

Command history is opt-in.

Enable it in Settings. New local terminals will include the history hook. Backend storage also checks consent, so emitted OSC-7337 events are ignored while disabled.

Existing shells may need to be restarted before hooks are installed.

## `agg` GIF export is unavailable

Open Settings and install GIF Export (`agg`). Mimir downloads a pinned `agg` release and verifies SHA256/size before installing it into the local Mimir config bin directory.

You can also install `agg` yourself and make sure it is on PATH.

## Update check says repository is not configured

Development builds default to:

```text
AppVersion=0.0.0-dev
UpdateRepository=
```

Release builds should set:

```powershell
.\scripts\release-all.ps1 -Version 0.1.0 -UpdateRepository OWNER/REPO
```

## Linux build dependency errors

Install Wails native dependencies:

```bash
sudo apt-get update
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.0-dev pkg-config build-essential
```

Runtime users usually need:

```bash
sudo apt install libgtk-3-0 libwebkit2gtk-4.0-37
```

Package names can differ by distribution/version.

## Svelte build warnings

Current known warnings:

- interactive `div` elements without ARIA role in a few UI controls
- chunk size above Vite default warning threshold

They do not fail the build. They should be cleaned up before a polished public release.
