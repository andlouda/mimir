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
  password combined with a machine-bound secret. A FIDO2 authenticator can be
  enrolled as an alternative unlock method (key *or* password). The master
  password remains as a recovery path if a hardware key is lost.
- Terminal recordings contain raw terminal data and may include secrets.
  Keystroke (input) recording is disabled by default. Scrubbing is best-effort
  and applied only when exporting; it cannot guarantee removal of free-form
  secrets such as an interactively typed password.
- Update checks are manual and use GitHub Releases. Installable auto-updates are intentionally not enabled yet.

See [docs/security-notes.md](docs/security-notes.md) for additional details.
