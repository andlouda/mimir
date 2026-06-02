# Security Notes

## Current Security Posture

Mimir is a local desktop app. It runs with the permissions of the logged-in user and intentionally has access to shells, SSH sessions, files selected through the UI, recordings, and local configuration.

The main security goals are:

- avoid hidden remote or shell mutation
- keep command history opt-in
- keep AI context explicit and sanitized
- verify downloaded helper binaries
- keep update installation manual until a signed updater exists
- avoid committing local user data

## Key Areas

| Area | Current behavior | Residual risk |
|------|------------------|---------------|
| Command history | Opt-in; backend enforces consent before storing OSC-7337 commands. | Shell hooks still execute when enabled and can expose command text locally. |
| SSH host keys | Unknown/mismatch keys require user accept/reject. | Users can still accept a malicious key. |
| SSH secrets | System keyring preferred; encrypted file fallback if unavailable. | File fallback is less strong than OS keyring. |
| SSH RC handling | Per-profile opt-in; does not write to remote `~/.bashrc`. | Local RC snippets may contain commands with side effects. |
| tmux SSH reconnect | tmux is used when available and profile enables it. | Existing tmux sessions keep their prior shell environment. |
| Templates | Local templates execute commands in the selected terminal. | Importing/running untrusted templates is equivalent to running untrusted shell commands. |
| AI prompts | Terminal context can be disabled and is sanitized when included. | Sanitizers reduce risk but cannot prove all secrets are removed. |
| Recordings | Recordings are local; scrubbed export exists. | Raw GIF/export can still contain sensitive terminal data if user chooses it. |
| `agg` helper | Built-in download is version-pinned, SHA256 checked, size-limited, timeout-limited. | Trust still depends on the pinned upstream release. |
| Updates | Manual GitHub Releases check only; no auto-install. | User must validate/install downloaded artifacts. |

## Local Data

Common local paths:

```text
<UserConfigDir>/mimir/mimir_session.json
<UserConfigDir>/mimir/transcripts/
<UserConfigDir>/mimir/ssh_profiles.json
<UserConfigDir>/mimir/known_hosts
<UserConfigDir>/mimir/ssh_secrets.enc
<UserConfigDir>/mimir/command_history.db
<UserConfigDir>/mimir/recordings/
<UserConfigDir>/mimir/notes/
<UserConfigDir>/mimir/bin/
```

These files should not be committed or shared without review.

## Command History

History capture uses a shell hook that emits an OSC-7337 escape sequence. The backend strips that sequence before output reaches xterm.

Storage requires two things:

1. hook emission from a shell/session
2. backend consent check enabled

The backend consent check is authoritative. If history is disabled, parsed OSC-7337 commands are ignored.

## SSH RC Modes

RC handling is per SSH profile:

- `off`: no Mimir RC behavior
- `remote-default`: start remote interactive shell
- `mimir`: start minimal clean bash/zsh/sh
- `local-snippet`: read a local file, limit to 64 KiB, upload to `~/.cache/mimir/shell/...`, and launch bash with `--rcfile`

Mimir never edits the remote user's `~/.bashrc`.

Use `local-snippet` only for hosts you control. It can execute arbitrary shell code.

## AI Context

AI settings include whether terminal output is included in prompts.

When terminal context is disabled, terminal output and redaction details are not sent to the provider prompt.

When enabled, the context pipeline records audit metadata such as:

- raw/sanitized character counts
- truncation
- provider
- whether context was included
- redaction counts by rule

Raw prompt content should not be written to audit metadata.

## Update Safety

The app checks GitHub Releases for update availability. It does not replace its own binary.

Release artifacts should include:

- platform artifact
- `checksums.txt`

Auto-install should not be added until there is a dedicated updater process and signature/checksum validation for every platform.

## Open Source Hygiene

Before pushing public:

- confirm `.gitignore` excludes `build/release`, `frontend/node_modules`, `frontend/dist`, local DBs, recordings, env files, and binaries
- do not commit real SSH profiles, known hosts, secrets, recordings, transcripts, or history databases
- review issue templates for secret redaction guidance
- enable GitHub Security Advisories
