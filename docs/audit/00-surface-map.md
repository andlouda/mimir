# 00 — Surface map

Frontend root: `app/frontend/src`. Entry `main.js` → `App.svelte` (global
shortcuts, session restore, modals host) → `lib/AppMainContent.svelte`
(page switch) + `lib/Sidebar.svelte`.

## Pages (`currentPage` in `stores/uiStore.js`, switched in `AppMainContent.svelte`)

| Page | Component | Reads (stores) | Backend events / bindings | Notes |
|---|---|---|---|---|
| terminals | `views/TerminalsPage.svelte` → `SplitPane.svelte` (recursive) | terminalStore (`terminals`, `activeTerminalId`, `layoutTree`), uiStore, agentStore | `terminal-output-<id>`, `terminal-closed-<id>`, `terminal-disconnected-<id>`, `terminal-prompt-<id>`, `agent-state-<id>`; `WriteToTerminal`, `ResizeTerminal`, tmux/agent bindings | xterm.js 6 per leaf; agent badge + restore markers in the pane header; context menu drawn by Mimir |
| fileBrowser | `FileBrowser.svelte` | sshStore (`fileBrowserRemoteTerminalId`) | SFTP bindings (`RemoteListDirectory`, read/upload) | remote only via SSH terminal |
| workflowBuilder | `workflows/WorkflowBuilder.svelte` (+ `WorkflowPlaybooksPane`, `WorkflowFunctionCatalogPane`, `WorkflowRunSummary`) | templateStore (playbooks), aiStore | workflow bindings; approval via `workflows/WorkflowApprovalDialog.svelte` | |
| aiHub | `views/AIHubView.svelte` | aiStore | AI bindings | |
| activityLogs / historyDashboard / recordings | `ActivityLogViewer.svelte`, `HistoryDashboard.svelte`, `RecordingPlayer.svelte` | — | activitylog / history / recording bindings | audit sub-pages under the AUDIT sidebar heading |
| settings | `views/SettingsView.svelte` | uiStore (tmux mode, renderer, font size), agentStore (detection, notifications, hook hosts) | `SetTmuxIntegrationMode`, `IsAgentDetectionEnabled`, hook install bindings, `GetGPUPolicy`/`SetGPUPolicy`, `GetWaylandFix` (Linux) | cards; some need restart |
| templateManager | `TemplateManager.svelte` | templateStore | template bindings | |
| agentWorkspace | `views/AgentWorkspaceView.svelte` | agentWorkspaceStore (`agentWorkspace`, `liveWorkspace`, `attentionItems`), agentStore, agentAnnotations | `GetAgentWorkspaceJSON`, `Save…Project/Task`, `Link/Unlink…Session`, `ListAgentSessionsJSON` | reached via "+" on AGENTS |

## Side panels and overlays

| Surface | Component | Opened by | State |
|---|---|---|---|
| Agent panel | `AgentTranscriptPanel.svelte` | pane badge, AGENTS row | `agentPanelTerminalId`, `agentPanelPinned`; tabs snippets/state/tasks/files/commands/processes/history/screen |
| Notes | `MarkdownNotes.svelte` | Ctrl+Shift+N, "to notes" actions | `notesPanelOpen`, `notesPanelWidth` (drag) |
| Sidebar | `Sidebar.svelte` | always; collapsible to icon rail | terminals folders (`customFolders`), SSH profiles, AGENTS (groups by project, collapsible, archived toggle), AUDIT submenu |
| Terminal search | in `SplitPane.svelte` via `actions/terminalSearchActions.js` | Ctrl+Shift+F | SearchAddon |
| Context menu | `SplitPane.svelte` | right click in a pane | copy / joined / link / tmux buffer / Shift hint |
| Secret gate | `SecretUnlockGate.svelte` | at start when keyring absent | blocks UI until unlocked |

## Modals (`AppModals.svelte` host, `lib/modals/*`)

SSHProfileModal (profile editor: password/key/jump host), HostKeyModal (TOFU),
TemplatePicker (Ctrl+Shift+P), TemplatePromptModal (variables), WorkflowPicker
(Ctrl+Shift+W), WorkflowApprovalDialog, AIPanelModal, AISettingsModal,
FunctionCatalogModal, AggDownloadModal (GIF export), DotEnvViewerModal,
TranscriptViewerModal. Update dialog lives in Settings (`updateStore.js`,
`update-progress` event).

## Stores (`lib/stores`)

terminalStore (terminals, activeTerminalId, layoutTree, terminalMap),
uiStore (currentPage, tmuxIntegrationMode, errorMessage, notes panel,
history consent, transcript viewer, dotenv viewer, terminalRenderer,
terminalFontSize), sshStore, aiStore, templateStore, sessionStore (folders),
updateStore, agentStore (agentStates, panel id/pin, detection, notifications,
annotations), agentWorkspaceStore.

## xterm.js integration (`actions/terminalActions.js`, `terminals/xtermLifecycle.js`)

Addons on this branch (main as of 2026-10-04): fit, search, clipboard
(write-only OSC 52 provider), link provider. unicode11 and webgl (renderer
setting) and the bundled JetBrains Mono arrive with PR #48, not merged yet;
lineHeight 1.35, font list JetBrains Mono / Fira Code / Cascadia Code /
monospace with system fallback. Link provider (`terminals/terminalLinks.js`,
multi-row links). Options: JetBrains Mono (bundled), lineHeight 1.35,
scrollback 100000, overviewRuler. Custom key handler cancels global
shortcuts. Resize: ResizeObserver → fit → debounced `ResizeTerminal`
(`RESIZE_SETTLE_MS = 300`). Wheel in tmux "invisible" mode → tmux copy-mode
keys (`SplitPane.svelte`). Paste routed through xterm (bracketed paste).

## Keyboard shortcuts (`actions/keyboardShortcuts.js`, window keydown)

Ctrl+Tab / Ctrl+Shift+Tab cycle; Ctrl+Shift+←/→ cycle; Ctrl+Shift+1…9 jump
(restore if minimized); Ctrl+Shift+M minimize; Ctrl+Shift+O (and U off
Linux) restore last minimized; Ctrl+Shift+T new terminal; Ctrl+Shift+N
notes; Ctrl+Shift+F search; Ctrl+Shift+P templates; Ctrl+Shift+W workflow
picker; Ctrl + / − / 0 and Ctrl+wheel zoom; Ctrl+Shift+C / Ctrl+Insert copy
selection (xterm handler); Escape closes overlays.

## Backend events consumed

`terminal-output-<id>`, `terminal-closed-<id>`, `terminal-disconnected-<id>`,
`terminal-prompt-<id>` (cwd beacon), `agent-state-<id>`, `update-progress`.

## Test setup

vitest (jsdom for sanitizer tests; 178 unit tests), Playwright (11 specs,
mocked backend, `npm run test:e2e`, serves a production build on
127.0.0.1:4173). No `wails dev` in this session; Playwright MCP tools exist
but the app needs the mock fixture, so agents use throw-away specs.

## Platform notes

Linux: WebKitGTK, GPU policy default "never" (setting), Wayland DMA-BUF
renderer disabled by default (setting), input-method fix for dead keys
(`configureInputMethod`). Windows: WebView2, ConPTY, PowerShell/cmd without
tmux. macOS: WebKit. i18n: EN/DE in `lib/locales/{en,de}.js` (876 lines
each, 26 top-level blocks); `i18n.js` falls back to the key string when a key
is missing.
