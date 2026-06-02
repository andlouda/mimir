# Contributing

Mimir is pre-MVP. Keep changes small, testable, and explicit about security impact.

## Branches

Use a simple GitHub flow:

- `main` is the default branch.
- Create feature/fix branches from `main`.
- Open pull requests back into `main`.

Suggested branch names:

```text
feature/ssh-rc-modes
fix/history-consent
docs/release-guide
```

## Commits

Conventional Commits are preferred:

```text
feat(ssh): add profile rc mode
fix(history): enforce backend consent
docs: update release workflow
test(update): cover version comparison
```

## Required Checks

Before opening a PR:

```bash
go test ./...
cd frontend
npm run build
```

There is currently no configured frontend test runner, ESLint, Prettier, or golangci-lint gate. Do not claim those checks ran unless you added or configured them.

## Go Style

- Run `gofmt` on edited Go files.
- Prefer small package-level helpers over large binding methods.
- Use structured parsing/APIs where available.
- Keep storage writes atomic where user data is involved.
- Do not log secrets, API keys, passwords, private keys, raw AI prompts, or raw terminal data unless explicitly required and sanitized.

## Frontend Style

- Follow the existing Svelte 5 style.
- Prefer small components under `frontend/src/lib/` for new feature-heavy UI.
- Use stable dimensions for terminal controls and badges.
- Keep Settings and Sidebar dense and operational.
- Do not edit `frontend/wailsjs/` manually.

## Security Review Checklist

Pay special attention when touching:

- SSH profiles, host keys, secrets, and RC behavior
- command history capture
- recordings and exports
- AI prompt/context handling
- update/download logic
- shell/template execution

PRs touching those areas should describe:

- what data is read/written
- where it is stored
- whether it can leave the machine
- how user consent is enforced

## Release Process

See [docs/releasing.md](docs/releasing.md).

Short version:

```bash
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions builds and publishes release artifacts.
