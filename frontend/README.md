# frontend

## Purpose

Svelte 5 frontend for the Wails desktop application. It renders terminal panes with xterm.js and manages navigation, settings, SSH profiles, templates, notes, recordings, history, AI, audit, and workflow views.

## Stack

| Dependency | Version |
|------------|---------|
| Svelte | `^5.55.10` |
| Vite | `^8.0.14` |
| `@sveltejs/vite-plugin-svelte` | `^7.1.2` |
| `@xterm/xterm` | `^6.0.0` |
| `@xterm/addon-fit` | `^0.11.0` |
| `@xterm/addon-search` | `^0.16.0` |
| `marked` | `^18.0.4` |

## Important Files

| Path | Purpose |
|------|---------|
| `src/App.svelte` | Main application shell and state orchestration. |
| `src/lib/SplitPane.svelte` | Split terminal layout and terminal header badges. |
| `src/lib/FileBrowser.svelte` | Local/remote file browser. |
| `src/lib/TemplateManager.svelte` | Template editor. |
| `src/lib/MarkdownNotes.svelte` | Notes panel. |
| `src/lib/HistoryDashboard.svelte` | Command history view. |
| `src/lib/RecordingPlayer.svelte` | Recording playback/export. |
| `src/lib/ActivityLogViewer.svelte` | Audit/activity log view. |
| `src/lib/workflows/WorkflowBuilder.svelte` | Workflow UI. |
| `src/lib/terminals/` | xterm, tmux, and layout helpers. |
| `wailsjs/` | Generated Wails bindings. Do not edit manually. |

## Commands

```bash
npm install
npm run dev
npm run build
```

Normally development is started from the repo root with:

```bash
wails dev
```

## WSL / Windows Dependency Note

Do not reuse the same `node_modules` between Windows and WSL. Native Vite/Rolldown packages differ by platform.

If the build reports a missing native binding:

```bash
rm -rf node_modules package-lock.json
npm install
```

## Current Limitations

- No frontend unit/integration test framework is configured yet.
- `App.svelte` is still large and should continue to be split into focused components.
- Production build currently emits known Svelte a11y warnings for a few interactive `div`s.
