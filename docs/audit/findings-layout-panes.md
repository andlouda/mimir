# Findings: layout-panes

Split panes, folders, pane headers, minimize/restore, side-panel widths,
window size, z-index stacking. Reproduced with the Playwright fixture
(`installMimirMocks`, viewport via `page.setViewportSize`, terminals added
through the UI); live-repro steps below refer to that setup. Backend resize
round trips, real PTY output and the Wails window itself are mocked, so
findings about `main.go` are code reasoning.

Counts: P1 ×2, P2 ×8, P3 ×6.

### [P1] "+ New" and session restore wrap the whole layout in a new split, so panes shrink exponentially
- Area: app/frontend/src/lib/actions/terminalActions.js:441
- Platform: all
- Symptom: every new terminal takes the right half of the whole area and the existing layout is squeezed into the left half. With 6 terminals the first two panes are 29 px wide, four of six panes have their close/minimize buttons clipped away; with 10 terminals three panes are 0 px wide (xterm has 0 columns). The same chain is rebuilt on every restart.
- Repro: 1. 1280×800, open Terminals. 2. Click "+ New" five times. 3. Pane widths are 29/29/63/129/262/528 px; the DOM tree is `h[h[h[h[h[t1,t2],t3],t4],t5],t6]`. 4. Minimize/close buttons of panes 1–4 are not hit by `elementFromPoint`. Confidence high.
- Cause: `addTerminal` always does `layoutTree.set({type:'split', direction:'horizontal', ratio:0.5, children:[tree, newLeaf]})` (terminalActions.js:441–446); `restoreLocalTerminal` (App.svelte:377, :401) and `terminalToForeground` (terminalActions.js:617–622) do the same, so n terminals yield widths 1/2, 1/4, … 1/2ⁿ⁻¹.
- Fix sketch: insert the new leaf beside the active leaf (`replaceLeaf` as in `splitTerminal`) or, when appending at the root, rescale the ratio to 1/(n+1) so every top-level pane keeps an equal share; cap automatic splits and fall back to opening the terminal minimized when a pane would drop under a pixel minimum.
- Evidence: terminalActions.js:436–447, App.svelte:372–377, docs/audit/shots/layout-panes-chain.png

### [P1] Split layout (direction, ratio, order) is never persisted; a restart rebuilds a horizontal chain
- Area: app/frontend/src/lib/actions/sessionActions.js:24
- Platform: all
- Symptom: after an update or restart (restore flow) every terminal comes back as a left-to-right chain in save order; a carefully arranged 2×2 grid or top/bottom split is lost each time.
- Repro: code reasoning, confidence high. `persistTerminalState` sends only id/type/name/minimized/sshProfileId/tmux/resumeId/restoreClass/folderId (sessionActions.js:26–36); `layoutTree` is imported there but never written. `restoreLocalTerminal` (App.svelte:362–377) rebuilds the tree with the same root-wrapping split as the previous finding. Divider drags (`touchLayoutTree`, AppMainContent.svelte:134) and pane moves (dragDrop.js:44) change the tree without any save.
- Cause: the session model is a flat terminal list; there is no layout serialisation in `UpdateTerminalState`/`SaveCurrentSession`.
- Fix sketch: serialise `layoutTree` (terminal ids mapped to resumeIds/positions) in the session save and rebuild it in the restore path, falling back to the chain only when no layout is stored.
- Evidence: sessionActions.js:2, :24–38; App.svelte:362–377; terminals/layoutTree.js

### [P2] Pane header controls are clipped off the right edge in narrow panes; close and minimize vanish first
- Area: app/frontend/src/style.css:164 (`.header-controls`)
- Platform: all
- Symptom: as a pane gets narrower the 7-button control group (≈ 170 px) is pushed beyond the pane's right edge and clipped by the parent's `overflow: hidden`. The rightmost buttons, minimize and close, disappear first while transcript/record/split stay visible. The pane can then only be closed via the context menu.
- Repro: 1. 1280×800, "+ New" once. 2. Drag the divider fully left (clamped at ratio 0.1 → 106 px pane). 3. `.header-controls` right edge is at 388 px while the pane ends at 326 px; close/minimize are not reachable. Also at 480×400 with two panes (128 px each) neither pane has a reachable close button. Confidence high.
- Cause: `.header-controls { flex-shrink: 0 }` (style.css:164–168) inside `.terminal-header` with `justify-content: space-between`; `.split-pane { overflow: hidden }` (SplitPane.svelte:645–650). `clampSplitRatio` (terminals/splitPaneResize.js:1–3) enforces only a relative 10 % minimum, which is 106 px at 1060 px but 40 px at 400 px.
- Fix sketch: enforce a pixel minimum per leaf in `calculateSplitRatio` (container size × ratio ≥ ~200 px); collapse the secondary buttons into a "…" menu below ~260 px and keep close/minimize always visible (order them first or `position: sticky; right: 0`).
- Evidence: style.css:110–120, :164–168; SplitPane.svelte:645–672; splitPaneResize.js:1–3; docs/audit/shots/layout-panes-chain.png

### [P2] Minimize → restore loses the pane's position and orientation
- Area: app/frontend/src/lib/actions/terminalActions.js:613
- Platform: all
- Symptom: a pane minimized from the top half of a top/bottom split comes back as the right half of a left/right split, with the other pane moved to the left. In a 3-pane layout the restored pane is appended at the far right as a new root split.
- Repro: 1. Split down (second split button). 2. Minimize the top pane. 3. Ctrl+Shift+O (or Ctrl+Shift+2 / sidebar click). 4. Root is now `split-horizontal`, order `[terminal-2, terminal-1]`, boxes 528 px wide side by side. With `h[v[t1,t2],t3]`, minimizing t2 and pressing Ctrl+Shift+2 yields `h[h[t1,t3],t2]`. Confidence high.
- Cause: `terminalToBackground` removes the leaf without remembering where it was (terminalActions.js:598); `terminalToForeground` always re-adds it as `{split horizontal, [tree, newLeaf]}` (613–623).
- Fix sketch: store the leaf's path (parent direction, index, sibling id, ratio) on the terminal when minimizing and re-insert next to that sibling via `replaceLeaf`; fall back to splitting the active leaf instead of the root.
- Evidence: terminalActions.js:589–623; App.svelte:121–127

### [P2] Notes/agent panel widths are not clamped to the window; the terminal area can shrink to 0 px
- Area: app/frontend/src/App.svelte:261
- Platform: all
- Symptom: a notes width saved on a wide monitor (up to 800 px) hides every terminal on a narrower window; the pane headers, agent badges and dividers are then under the notes panel. Even at the default 1228×922 window, notes (380) + agent panel (440) + sidebar (220) leave 183 px for the terminal: the pane title is 0 px wide and xterm reports 1 column.
- Repro: 1. `localStorage['mimir-notes-width']='800'`, viewport 1024×700, Ctrl+Shift+N. 2. `.terminal-area` width is 0, notes 800 px. 3. Open the agent panel (badge): agent 402 px, notes 800 px, area 0 px; `elementFromPoint` over the agent badge hits `.notes-title`. 4. 1228×922 with default widths and both panels: area 183 px, `.terminal-name` 0 px, 1 column. Confidence high.
- Cause: `startNotesDrag` clamps only to 250–800 (App.svelte:261); `initialNotesPanelWidth` (uiStore.js:3–9) reads the stored value unclamped; `.notes-panel` (`min-width: 250px; max-width: 800px`) and `.agent-panel` (`width: 440px; min-width: 300px; max-width: 50%`) are `flex-shrink: 0` (main.css:194–211), while `.terminal-area` has `min-width: 0` (main.css:178).
- Fix sketch: clamp both panel widths against `innerWidth − sidebar − other panel − ~420 px` on load, on drag and on window resize; give `.terminal-area` a real `min-width` and let the panels shrink (or auto-collapse the notes panel) before the terminal does.
- Evidence: App.svelte:256–270; uiStore.js:3–9; main.css:172–211; docs/audit/shots/layout-panes-squeeze.png

### [P2] Restored-transcript summary stacks above modals and covers the search bar
- Area: app/frontend/src/lib/SplitPane.svelte:839 (`.restore-summary { z-index: 60 }`)
- Platform: all
- Symptom: on a session-restored cmd/PowerShell pane the "Restored transcript" box stays on top of the Templates/Settings modal backdrop and sits exactly over the Ctrl+Shift+F search bar, so the search input cannot be clicked.
- Repro: 1. Session with `restoreClass: 'transcript-restored'` and a transcript excerpt. 2. Ctrl+Shift+P: `elementFromPoint` over the summary returns `.restore-summary-title`; computed z-index summary 60 vs `.modal-overlay` 30. 3. Click the pane, Ctrl+Shift+F: the search input at (1023,88) is covered by `.restore-summary` (850–1270, 90–174). Confidence high.
- Cause: `.terminal-container` is `position: relative` without `z-index`/`isolation` (SplitPane.svelte:674–681), so the summary's z-index 60 and close button 61 compete in the root stacking context against `.modal-overlay` z-index 30 (styles/overlays.css:60) and `.search-bar` z-index 20 (SplitPane.svelte:983). The context menu (99/100) and the agent badge (2) live in the same flat space.
- Fix sketch: add `isolation: isolate` (or `z-index: 0`) to `.terminal-container` so pane-internal layers stay local; render the summary below the search bar (top offset when the search is open) or dock it at the bottom of the pane.
- Evidence: SplitPane.svelte:674–681, :835–871, :975–983; overlays.css:52–61

### [P2] All panes minimized: empty state says "No terminals open" and the sidebar hides the minimized terminals
- Area: app/frontend/src/lib/views/TerminalsPage.svelte:140
- Platform: all
- Symptom: after minimizing the last visible pane the page shows `No terminals open. Click "+ New" to create one.` and the counter reads `0 active`; the TERMINAL sidebar section is collapsed by default (`aria-expanded=false`, 0 rows), so the only way back is Ctrl+Shift+O or discovering the heading toggle. A user following the empty-state text creates an extra terminal and the old one keeps running unseen.
- Repro: 1. Fresh app, click "–" on the single pane. 2. `.empty-state` text = "No terminals open…", `.sidebar-subnav li` count 0, heading `aria-expanded=false`. Confidence high.
- Cause: `{#if layoutTree} … {:else} empty-state` ignores `terminals.length` (TerminalsPage.svelte:97–143); `terminalNavOpen = false` initial state (Sidebar.svelte:62) and the list renders only when open (:206).
- Fix sketch: when `terminals.length > 0 && visibleTerminalCount === 0` show "N terminal(s) minimized" with a Restore button (and the Ctrl+Shift+O hint); auto-expand the TERMINAL section when a terminal is minimized.
- Evidence: TerminalsPage.svelte:140–142; Sidebar.svelte:62, :195–206; docs/audit/shots/layout-panes-minimized-empty.png

### [P2] Dragging to select text in the rename input starts a pane drag instead
- Area: app/frontend/src/lib/SplitPane.svelte:335 (`draggable="true"` on `.terminal-header`)
- Platform: all
- Symptom: click the pane title to rename, then try to select part of the name with the mouse: a pane-drag ghost appears, the drop overlays show on the other panes, and the selection stays empty.
- Repro: 1. Click `.terminal-name`. 2. Mouse down at x+4 inside `.name-input`, move 40 px, mouse up. 3. `dragstart` fires once, `selectionStart === selectionEnd === 0`. Confidence high.
- Cause: the header is the HTML5 drag handle for the whole pane; the `<input>` (:370–378) inherits it, and nothing stops `dragstart`/`mousedown` for the editing state.
- Fix sketch: `draggable={!term.editingName}` on the header, plus `on:mousedown|stopPropagation` and `on:dragstart|preventDefault|stopPropagation` on the input; same for the search input inside the pane.
- Evidence: SplitPane.svelte:331–338, :370–378; actions/dragDrop.js:6–17

### [P2] No minimum window size; the collapsed sidebar rail clips its lower icons and the controls bar overflows
- Area: app/main.go:54 (window options have no `MinWidth`/`MinHeight`)
- Platform: all
- Symptom: shrink the window (or snap it to a quarter screen) and the collapsed icon rail loses AI/Audit/Settings below the fold with no way to scroll; the controls bar clips the "AI:" and "N active" counters; two panes at 480 px are 128 px each with no reachable close button.
- Repro: 1. 640×360, click the sidebar toggle: collapsed icon bottoms are 99…489 px, `.sidebar` scrollHeight 497 vs clientHeight 360 and `overflow-y: hidden`. 2. 480×400 with two panes: `.terminal-controls` scrollWidth 285 > clientWidth 260. Confidence high for the DOM, medium for the real window (Wails default allows any size).
- Cause: no `MinWidth`/`MinHeight` in `options.App` (main.go:52–60); `.sidebar.collapsed { overflow-y: hidden }` (styles/sidebar.css:19–21); `.terminal-controls` has no wrap/overflow handling (styles/main.css:15–30).
- Fix sketch: set `MinWidth: 900, MinHeight: 560` (or similar) in `main.go`; keep `overflow-y: auto` with a hidden scrollbar on the collapsed rail; let `.controls-right` wrap or hide the AI label first.
- Evidence: main.go:52–60; sidebar.css:5–21; main.css:15–43

### [P2] Pane title is clipped to 0 px before any badge or button gives way
- Area: app/frontend/src/style.css:110 (`.header-left { overflow: hidden }`)
- Platform: all
- Symptom: in a three-pane layout at 1024 px two panes (198 px wide) show only a truncated "BA" type badge and no name at all; the user cannot tell which pane is which, while seven header buttons stay fully visible. With tmux/rc/agent/restore badges (all `flex-shrink: 0`) the name is gone even earlier.
- Repro: 1. 1024×700, "+ New" twice. 2. `.terminal-name` widths are 0/0/41 px; `.header-left` clientWidth 21 px, scrollWidth 47. 3. Rename pane 3 to a long title: it ellipsises at 177 px, panes 1–2 still 0 px. Confidence high.
- Cause: `.header-left { flex: 1; min-width: 0; overflow: hidden }` with every badge `flex-shrink: 0` (style.css:120–134, SplitPane.svelte:775–780) and `.header-controls { flex-shrink: 0 }` taking its full width first.
- Fix sketch: give `.terminal-name` a `min-width` (e.g. 6ch) and let badges collapse to dots via the existing container query at a wider breakpoint; collapse the button group before the name (see the controls finding).
- Evidence: style.css:110–150, :164–168; SplitPane.svelte:339–389, :773–780

### [P3] Split divider: 4 px hit target, no keyboard resizing despite `role="slider"`, no double-click reset
- Area: app/frontend/src/lib/SplitPane.svelte:585
- Platform: all
- Symptom: the divider is a 4 px strip that is hard to grab (especially with a touchpad or at 150 % DPI); it is focusable and announced as a slider, but Arrow keys, Home/End and double-click do nothing.
- Repro: 1. "+ New", focus `.split-divider`, press ArrowLeft twice: `aria-valuenow` stays 50. 2. Double-click: unchanged. Bounding box 4×751 px. Confidence high.
- Cause: only `on:mousedown={startDrag}` is bound (SplitPane.svelte:585–594); CSS width/height 4 px (:664–672); mouse-only listeners (no pointer events, so no touch).
- Fix sketch: add `on:keydown` (±2 % per arrow, Home/End to the clamps), `on:dblclick` → 0.5, switch to pointer events, and widen the hit area with a transparent `::after` while keeping the 4 px visual.
- Evidence: SplitPane.svelte:25–47, :585–594, :653–672

### [P3] Counters in the controls bar run together: "AI: OpenAI1 active"
- Area: app/frontend/src/styles/main.css:32 (`.controls-right`)
- Platform: all
- Symptom: the AI-provider label and the active-terminal count are rendered as one string ("AI: OpenAI6 active").
- Repro: 1. Open Terminals. 2. The two `.terminal-count` spans have a 0 px gap; computed `gap: normal`. Visible in all three screenshots. Confidence high.
- Cause: `.controls-right { display: flex; align-items: center }` without `gap` (main.css:32–35), unlike `.controls-left` which has `gap: 0.5rem`.
- Fix sketch: `gap: 1rem` or a separator; consider hiding the AI label under ~700 px.
- Evidence: main.css:26–43; TerminalsPage.svelte:88–91; docs/audit/shots/layout-panes-chain.png

### [P3] Sidebar: terminals after the ninth lose their digit badge and shift 31 px left
- Area: app/frontend/src/lib/Sidebar.svelte:233
- Platform: all
- Symptom: rows 1–9 show a `kbd` badge and the name starts at x=82; row 10 has no badge and its name starts at x=51, so the list looks ragged and the tenth terminal appears to belong to another group.
- Repro: 1. Ten terminals, expand TERMINAL. 2. `.sidebar-terminal-name` x offsets `[82×9, 51]`. Confidence high.
- Cause: `shortcutIndex` only covers the first nine (Sidebar.svelte:139) and the `kbd` is conditionally rendered (:233) with no placeholder.
- Fix sketch: always render the badge slot (empty, fixed width) or put the name in a fixed grid column.
- Evidence: Sidebar.svelte:138–139, :229–236; styles/sidebar.css:325–341

### [P3] Dragging a pane onto another pane's header, a divider or the toolbar does nothing and shows no feedback
- Area: app/frontend/src/lib/SplitPane.svelte:423
- Platform: all
- Symptom: the header is the drag handle, so users naturally drop on the target's header; nothing happens, no indicator, no "not allowed" cursor. Only the terminal body (below the header) accepts the drop.
- Repro: 1. "+ New". 2. Drag header 1 onto header 2: order unchanged, no `.drop-overlay` reaction. Confidence high.
- Cause: the `.drop-overlay` is rendered inside `.terminal-container` only (SplitPane.svelte:423–434); the header and the dividers have no `dragover`/`drop` handlers.
- Fix sketch: mount the overlay on `.terminal-wrapper` (covering the header) and compute the zone from the wrapper rect; treat a header drop as "swap".
- Evidence: SplitPane.svelte:415–434; actions/dragDrop.js:30–47

### [P3] Folder drag-and-drop: tiny drop targets and no drag handle until a folder exists
- Area: app/frontend/src/lib/Sidebar.svelte:211
- Platform: all
- Symptom: a terminal row can be dropped only on the folder's one-line header (`li.sidebar-folder`, ≈ 1.5 rem) or on the "Auto group" strip that appears at the bottom of the list while dragging; dropping onto the rows of the target folder does nothing. Rows are `draggable` only when a custom folder exists, so the affordance appears without explanation after the first folder is created.
- Repro: code reasoning, confidence medium (needs the Folder Manager modal; not driven in the fixture).
- Cause: `on:dragover/drop` are bound on the folder header `li` only (Sidebar.svelte:211–215); `draggable={customFolders.length > 0}` (:228); auto groups are not drop targets except via the `__auto__` strip (:243–256).
- Fix sketch: wrap each group (header + rows) in a container that is the drop target and highlight it; keep rows draggable always and show a "Create a folder to group terminals" hint when none exist.
- Evidence: Sidebar.svelte:206–257; actions/folderActions.js:56–63; terminals/sidebarGroups.js:31–47

### [P3] Auto-generated pane names collide after closing a terminal
- Area: app/frontend/src/lib/actions/terminalActions.js:425
- Platform: all
- Symptom: with BASH 1–3 open, close BASH 2 and click "+ New": the new pane is named "BASH 3" while another BASH 3 still exists; headers, sidebar rows and Ctrl+Shift+digit badges then show two identical names.
- Repro: code reasoning, confidence high (`${type.toUpperCase()} ${get(terminals).length + 1}`).
- Cause: the name is derived from the current list length, not from a per-type counter or the next unused number.
- Fix sketch: pick the smallest unused number per type (or use a monotonic counter persisted with the session).
- Evidence: terminalActions.js:417–426

## Not found / verified OK

- Context menu clamps to the viewport (bottom edge 801/800 px, right edge 1260/1280 px) and re-opens cleanly on a second right-click (SplitPane.svelte:148–151).
- Window resize: `resize` listener + rAF `handleResize` (App.svelte:428, terminalActions.js:407) and a per-terminal `ResizeObserver` (`observeTerminalResize`, terminalActions.js:343) keep panes fitted during divider drags; the sidebar toggle re-fits after its 220 ms transition.
- Closing a leaf inside a nested split collapses the parent node correctly (`removeLeafFromTree`), the sibling takes the whole slot, and the active id moves to a visible pane.
- Ctrl+Shift+1…9 badges match the sidebar order (`shortcutIndex`) and restore a minimized terminal (position aside, see the P2 above).
- Agent badge stays clickable in narrow panes (`flex-shrink: 0`, local z-index 2, text hidden via the `header-left` container query) — code reasoning, the mock did not surface a badge in the 3-pane run.
- No horizontal document scroll at 480 px; `.terminal-controls` does not overflow at 640 px.
- Drag-and-drop move between pane bodies reorients the split via `moveLeaf` (unit-tested in dragDrop.test.js / layoutTree); a 2-pane left-zone drop is correctly a no-op.
- Divider ratio clamp 0.1–0.9 and `aria-valuenow` reporting work (10 at the left stop).
