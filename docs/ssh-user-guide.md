# SSH User Guide

Mimir supports SSH as a first-class terminal type.

## Quick Start

1. Open `SSH` in the sidebar.
2. Create or edit an SSH profile.
3. Enter host, port, username, and auth method.
4. Keep `Use tmux when available` enabled for persistent sessions.
5. Connect.

## Profile Fields

| Field | Meaning |
|-------|---------|
| Name | Display name in the UI. |
| Host | Hostname or IP. |
| Port | SSH port, default `22`. |
| Username | Remote username. |
| Auth Method | Password or SSH key. |
| Key Path | Private key path when using key auth. |
| Password / Passphrase | Stored in secret storage, not in the profile JSON. |
| Use tmux when available | Enables tmux-first reconnect behavior. |
| RC Mode | Optional per-profile shell RC behavior. |

## Authentication

### Password

Passwords are stored through the secret store.

### SSH Key

Mimir can list common keys from `~/.ssh/`. It does not import or modify keys.

If the key is encrypted, enter the passphrase in the password/passphrase field.

## Secret Storage

Mimir prefers the system keyring:

| Platform | Backend |
|----------|---------|
| Windows | Credential Manager |
| macOS | Keychain |
| Linux | Secret Service |

If unavailable, Mimir falls back to an encrypted local file:

```text
<UserConfigDir>/mimir/ssh_secrets.enc
```

The UI shows a warning when the fallback is active.

## Host-Key Verification

Mimir checks SSH host keys with its local known-hosts store.

Unknown or mismatched keys show a verification dialog. Do not accept a mismatch unless you intentionally changed the host key.

## tmux

tmux is the default stability layer for SSH sessions.

When enabled:

- Mimir probes whether `tmux` exists on the host.
- If available, Mimir attaches/creates a deterministic session for the profile.
- Reconnect lands in the same tmux session.

Badges:

| Badge | Meaning |
|-------|---------|
| `tx` | tmux is active. |
| `no tx` | tmux is missing on the host. |
| `clean` | no Mimir RC injection is active. |
| `rc` | an RC mode is active for this profile. |

Install hints:

```bash
sudo apt install tmux
sudo dnf install tmux
sudo pacman -S tmux
sudo apk add tmux
```

## RC Modes

Default is `Off`.

| Mode | Behavior |
|------|----------|
| Off | Mimir does not add RC behavior. |
| Remote shell defaults | Starts the remote interactive shell. |
| Mimir clean RC | Starts a minimal clean bash/zsh/sh shell. |
| Local RC snippet | Uploads a local file to `~/.cache/mimir/shell/...` and starts bash with `--rcfile`. |

Mimir does not write to remote `~/.bashrc`.

Use `Local RC snippet` only for hosts you control. The snippet can execute arbitrary shell code.

If tmux reconnects to an existing session, RC mode cannot change that already-running shell. Start a fresh tmux session to test RC changes.

## Reconnect

If the SSH transport disconnects, Mimir marks the terminal disconnected and exposes reconnect behavior.

Best persistence requires:

- profile has tmux enabled
- remote host has tmux installed
- the tmux session still exists

Without tmux, reconnect creates a new remote shell.

## Remote Files

The file browser can operate against an SSH terminal for remote listing/file content where supported by the app UI.

## Stored Data

| Data | Location |
|------|----------|
| Profiles | `<UserConfigDir>/mimir/ssh_profiles.json` |
| Secrets fallback | `<UserConfigDir>/mimir/ssh_secrets.enc` |
| Known hosts | `<UserConfigDir>/mimir/known_hosts` |
| Uploaded RC snippets | Remote `~/.cache/mimir/shell/` |

Passwords are not stored in `ssh_profiles.json`.

## Troubleshooting

### Connection failed

Check host, port, DNS, firewall, username, auth method, and key path.

### Password not found

Edit the profile and save the password/passphrase again.

### Key parse failed

The key may be encrypted or unsupported. Try adding the passphrase.

### Host-key mismatch

Treat as suspicious unless you intentionally recreated/rekeyed the host.

### `no tx`

tmux is not installed or not in PATH on the remote host.

### RC mode has no effect

You may be reconnecting to an existing tmux session. Kill/create a new tmux session and reconnect.
