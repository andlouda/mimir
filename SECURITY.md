# Security Policy

## Supported Versions

Security fixes are handled on the latest released version.

## Reporting a Vulnerability

Please do not open public issues for vulnerabilities. Report them privately via GitHub Security Advisories once the repository is public.

If advisories are not enabled yet, contact the maintainer through the GitHub profile linked from the repository.

## Security Notes

Mimir is a local desktop terminal application. It may handle sensitive terminal output, SSH profile metadata, command history, recordings, and AI prompts.

- Command history is opt-in and stored locally. The OSC sequence used to
  capture commands is treated as untrusted input (any process writing to the
  terminal can emit it): payloads are length-capped and control characters are
  rejected. History captured from remote (SSH) sessions is inherently
  attacker-influenceable and should not be treated as a trusted record.
- AI context is sanitized before provider calls, but users should still review prompts before sending sensitive data.
- SSH host keys are verified through the local known-hosts store.
- Remote RC injection is opt-in per SSH profile and does not write to remote `~/.bashrc`.
- Credentials (SSH passwords and the AI API key) use the OS keyring when
  available. The fallback encrypted-file backend uses envelope encryption: a
  random data key is wrapped by a key derived (Argon2id) from a user master
  password combined with a machine identifier where the OS provides one
  (`/etc/machine-id`, IOPlatformUUID, MachineGuid). That identifier is a
  secondary layer only: it is readable by local users and is absent on some
  platforms, in which case derivation is password-only. A FIDO2 authenticator can be
  enrolled as an alternative unlock method (key *or* password). The master
  password remains as a recovery path if a hardware key is lost.
- Clipboard: programs may *write* the clipboard through OSC 52 (in plain
  terminals via xterm's clipboard addon, inside tmux sessions via
  `set-clipboard on`, which also passes on a nested tmux's selections);
  reads are denied, so no remote program can exfiltrate clipboard content.
  Pasting always goes through bracketed paste, so multi-line clipboard text
  is not executed line by line.
- The agent panel detects coding agents (Claude Code, Codex, ...) by
  inspecting the process tree of a pane's tmux session out-of-band (`ps`, never
  keystrokes). tmux is the primary source: `capture-pane` provides the pane
  text for every agent and is used to confirm which session file belongs to
  the pane. Session files (`~/.claude/projects`, `~/.codex/sessions`; over
  SFTP for SSH panes) are read only to obtain unwrapped text. Those files can contain
  anything the agent saw, including secrets; they are rendered through the
  same sanitizer as notes, never uploaded, and only metadata (agent kind,
  message count) is written to the activity log. Detection is on by default
  and can be disabled in Settings. On terminals without tmux (PowerShell,
  cmd, tmux mode off) the process tree is read from the terminal's own child
  process and the working directory from Mimir's prompt hook; enabling
  detection therefore counts as consent for injecting that hook (it only
  reports to Mimir, and the command line it carries is stored only when
  history tracking is on).
- Session names, notes and archived flags for agents are the only state Mimir
  keeps about them across restarts: one JSON file (mode 0600) in the config
  directory, containing what the user typed plus the host and session-file
  path as key. Projects are derived from the git root, never created; Mimir
  does not start agents or create worktrees.
- The optional Claude Code hooks are a `Notification` and a `SessionStart`
  entry in `~/.claude/settings.json` that Mimir adds on request and removes
  again on request (the SessionStart payload only names the session file
  and is used to bind a pane to its session); installing and removing it is written to the activity log and the
  file is left untouched when it is not valid JSON. Locally the entry runs
  `mimir --agent-hook` in exec form (no shell), which validates the payload
  and stores it under the user cache directory with mode 0600; on WSL and SSH
  hosts it is a `cat` one-liner into `~/.cache/mimir/agent-events`. Mimir
  reads those files only to mark the pane as waiting for approval and
  deletes them once consumed. The hook file name carries the agent's
  process id, so a prompt is attributed to the pane whose agent raised it.
  The Allow / Deny buttons type a single key ("1" or Escape) into that pane,
  only while the hook has reported a permission prompt there and only once
  per prompt; every answer is written to the activity log. Mimir never
  answers a prompt on its own.
- Terminal recordings contain raw terminal data and may include secrets.
  Keystroke (input) recording is disabled by default. Scrubbing is best-effort
  and applied only when exporting; it cannot guarantee removal of free-form
  secrets such as an interactively typed password.
- Update checks are manual and use GitHub Releases. Installable auto-updates are intentionally not enabled yet.

See [docs/security-notes.md](docs/security-notes.md) for additional details.
