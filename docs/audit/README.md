# UI/UX audit (2026-10-04)

Read-only audit of Mimir's frontend (Svelte 5, xterm.js 6, Wails v2 webviews).
Nothing under `app/` is changed by this audit; every result lives in this folder.

- `00-surface-map.md` — every screen, panel, modal, store, backend event and shortcut.
- `01-prior-art.md` — bug classes mined from resolved issues of comparable projects.
- `findings-<domain>.md` — one file per domain (see list below).
- `99-summary.md` — ranked summary across domains.

Domains: terminal-render, layout-panes, theme-dpi, i18n-copy, forms-modals,
side-panels, state-sync, keyboard-a11y, platform.

## Finding format (strict)

```
### [P1|P2|P3] <short title>
- Area: <component or file>:<line>  (one primary location)
- Platform: all | linux-webkitgtk | windows-webview2 | macos-webkit
- Symptom: what the user sees, one or two sentences
- Repro: numbered steps; or "code reasoning" with confidence low | medium | high
- Cause: the code path (file:line) that produces it
- Fix sketch: one to three lines, no code
- Evidence: file:line references; screenshot path under docs/audit/shots/ if any
```

Severity: P1 breaks a flow or loses data/state; P2 visibly wrong but workable;
P3 polish. Findings are ordered by severity, then confidence. Each file ends
with a short "Not found / verified OK" list so negative results are kept.

## Live reproduction

There is no `wails dev` session; Wails bindings are mocked. The Playwright
setup (`app/frontend/playwright.config.js`) builds the frontend and serves it
at http://127.0.0.1:4173 with `tests/e2e/fixtures/mimirApp.js`
(`installMimirMocks(page, { workspaceAgent })`) stubbing `window.go` and
`window.runtime`. Agents reproduced by writing throw-away specs
`tests/e2e/zz-audit-<domain>.spec.js`, running
`npx playwright test tests/e2e/zz-audit-<domain>.spec.js`, and deleting the
spec afterwards. What the mocks cannot do (real PTY output, resize round
trips to the backend, SSH) is marked as code reasoning.
