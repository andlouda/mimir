# Repository Map

## Root

| Path | Purpose |
|------|---------|
| `main.go` | Wails app entrypoint and embedded assets/templates. |
| `app.go` | App construction, shared state, startup/session lifecycle. |
| `app_*.go` | Feature-specific Wails bindings: SSH, SFTP, history, notes, recordings, folders, updates. |
| `ai.go` | AI settings, provider calls, prompt/tool orchestration. |
| `workflow_runtime.go` | Runtime bridge for workflow execution. |
| `playbooks_api.go` | Wails API for playbooks/workflows. |
| `version.go` | Build-time version and update repository variables. |
| `wails.json` | Wails project config. |
| `go.mod`, `go.sum` | Go module definition and dependency lock. |
| `LICENSE` | MIT license. |
| `SECURITY.md` | Vulnerability/security policy. |
| `README.md` | Public project overview. |

## Backend Packages

| Path | Purpose |
|------|---------|
| `activitylog/` | Local activity/audit log storage. |
| `aiflow/` | AI tool-flow config, context sanitization, guardrails, discovery. |
| `folder/` | Custom terminal sidebar folders. |
| `history/` | Command history SQLite store and OSC parser. |
| `notes/` | Notes storage. |
| `recording/` | Asciicast recorder, scrubber, export, pinned `agg` download. |
| `safeio/` | Atomic file writes. |
| `session/` | Persisted terminal/session state. |
| `ssh/` | SSH profiles, keys, known hosts, secrets. |
| `template/` | Command template loading, editing, execution. |
| `terminal/` | Local/SSH terminal session management. |
| `tools/` | Tool registry and template tool bridge. |
| `transcript/` | Terminal transcript persistence. |
| `update/` | GitHub Releases update check, download, SHA256 verify, staged install. |
| `workflow/` | Workflow engine, approvals, playbooks, validation. |

## Frontend

| Path | Purpose |
|------|---------|
| `frontend/src/App.svelte` | Main app shell, navigation, terminal orchestration, settings, modals. |
| `frontend/src/lib/SplitPane.svelte` | Split-pane terminal layout and terminal header badges. |
| `frontend/src/lib/FileBrowser.svelte` | Local/remote file browser. |
| `frontend/src/lib/TemplateManager.svelte` | Template CRUD UI. |
| `frontend/src/lib/MarkdownNotes.svelte` | Notes UI. |
| `frontend/src/lib/HistoryDashboard.svelte` | Command history dashboard. |
| `frontend/src/lib/RecordingPlayer.svelte` | Recording playback/export UI. |
| `frontend/src/lib/ActivityLogViewer.svelte` | Activity/audit log viewer. |
| `frontend/src/lib/workflows/WorkflowBuilder.svelte` | Workflow builder UI. |
| `frontend/src/lib/workflows/WorkflowPlaybooksPane.svelte` | Playbook cards with run buttons. |
| `frontend/src/lib/workflows/WorkflowRunSummary.svelte` | Workflow execution results display. |
| `frontend/src/lib/modals/WorkflowPicker.svelte` | Ctrl+Shift+W command palette for workflows. |
| `frontend/src/lib/modals/HostKeyModal.svelte` | SSH host key verification TOFU modal. |
| `frontend/src/lib/modals/TemplatePicker.svelte` | Ctrl+Shift+P command palette for templates. |
| `frontend/src/lib/views/SettingsView.svelte` | Settings page with update, shortcuts, folders. |
| `frontend/src/lib/views/AIHubView.svelte` | AI configuration and interaction hub. |
| `frontend/src/lib/AppModals.svelte` | Central modal orchestration. |
| `frontend/src/lib/AppMainContent.svelte` | Main content area routing. |
| `frontend/src/lib/Sidebar.svelte` | Navigation sidebar. |
| `frontend/src/lib/terminals/` | xterm lifecycle, tmux helpers, layout tree helpers. |
| `frontend/wailsjs/` | Generated Wails bindings. Do not edit manually. |
| `frontend/dist/` | Generated Vite output. Ignored by Git. |

## Configuration And Data

| Path | Purpose |
|------|---------|
| `config/ai_tool_flow.yaml` | Default AI tool-flow config. |
| `templates/` | Embedded command template JSON files. |
| `build/` | Wails build metadata, icons, manifests, installer config. |
| `build/bin/` | Generated binaries. Ignored by Git. |
| `build/release/` | Local release artifacts/checksums. Ignored by Git. |

## Documentation

| Path | Purpose |
|------|---------|
| `docs/architecture.md` | Current architecture overview. |
| `docs/development.md` | Local development guide. |
| `docs/testing.md` | Test strategy and commands. |
| `docs/security-notes.md` | Security model and residual risks. |
| `docs/releasing.md` | Release/update workflow. |
| `docs/local-release.md` | Local artifact build commands. |
| `docs/ssh-user-guide.md` | SSH profile usage. |
| `docs/reboot-workspace-restore.md` | Feature concept for restoring admin workspaces after app restarts and machine reboots. |
| `docs/transcript-module.md` | Architecture and data flow of the per-terminal transcript store + viewer. |
| `docs/adr/` | Architecture Decision Records. |

## GitHub

| Path | Purpose |
|------|---------|
| `.github/workflows/ci.yml` | Go tests and frontend build. |
| `.github/workflows/release.yml` | Cross-platform artifacts and GitHub Release publishing. |
| `.github/ISSUE_TEMPLATE/` | Bug/feature issue templates. |
| `.github/pull_request_template.md` | PR checklist. |

## Generated Or Local-Only

Do not commit:

- `frontend/node_modules/`
- `frontend/dist/`
- `build/bin/`
- `build/release/`
- local databases
- recordings
- transcripts
- env files
- release archives
