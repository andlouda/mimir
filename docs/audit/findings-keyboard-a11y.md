# Findings — keyboard & accessibility

Scope: `actions/keyboardShortcuts.js` (window keydown), the xterm custom key
handler in `actions/terminalActions.js`, Escape handling, focus order and
restoration, context menu, modal keyboard behaviour, aria roles/names,
reduced motion. Live reproduction with the Playwright fixture (Chromium,
mocked backend) through a throw-away spec (deleted); everything the fixture
cannot show (native context menu, WebKitGTK/WebView2 accelerators, real
shells) is marked as code reasoning with a confidence.

Key mechanism behind several findings: xterm.js calls
`cancel(event, true)` (preventDefault **and** stopPropagation) for every
keydown it handles itself. Mimir's global handlers hang on `window`
(`App.svelte:507`, `SplitPane.svelte:320`), so they only see a key when the
custom key handler (`terminalActions.js:136-151`) has declared it a global
shortcut. Escape is not in that list, so the window-level Escape logic never
runs while a terminal has focus — which is the default state after opening
the search bar or the context menu.

---

### [P1] Escape never reaches Mimir while a terminal has focus — it goes to the shell program instead and the overlay stays open
- Area: `app/frontend/src/lib/actions/keyboardShortcuts.js:197` (Escape branch) and `app/frontend/src/lib/SplitPane.svelte:235` (context-menu Escape)
- Platform: all
- Symptom: Ctrl+Shift+F opens the search bar (focus stays in the terminal, see next finding); the user presses Escape to dismiss it — the bar stays, and `\x1b` is written to the PTY. Inside Claude Code that cancels the running turn, in vim it leaves insert mode, in a readline prompt it starts a meta sequence. Same for the right-click context menu: Escape does not close it, the keystroke goes to the shell.
- Repro:
  1. Click into terminal 1, press Ctrl+Shift+F (search bar appears, `document.activeElement` is still `textarea.xterm-helper-textarea`).
  2. Press Escape. Observed: `.search-bar` count stays 1, `WriteToTerminal(1, "\u001b")` is called.
  3. Right-click the terminal, press Escape. Observed: `.ctx-menu` count stays 1, focus still in the xterm textarea.
  4. Control: focus the sidebar toggle button, press Escape → the search bar closes (window handler works when xterm does not own the key).
- Cause: `isGlobalShortcut()` (`keyboardShortcuts.js:30-38`) returns false for Escape, so the custom key handler returns true, xterm processes the key and stops propagation; the `svelte:window on:keydown` handlers in `App.svelte:507` and `SplitPane.svelte:320` are never invoked.
- Fix sketch: handle Escape in the xterm custom key handler when any Mimir overlay (search, context menu, restore summary, picker) is open — return false and dispatch the close — or register the window listener in the capture phase. Alternatively move focus into the overlay when it opens (next finding), which makes Escape land on the overlay element.
- Evidence: `terminalActions.js:136-151`, `keyboardShortcuts.js:197-210`, `SplitPane.svelte:234-238`; xterm.js `cancel(e,!0)` in `node_modules/@xterm/xterm/lib/xterm.js`. Live log: `search after Escape: 1 writes: ["1:\"\\u001b\""]`, `ctx after Escape: 1`.

### [P1] Ctrl+Shift+F shows the search bar but leaves focus in the terminal: the query is typed into the shell
- Area: `app/frontend/src/lib/SplitPane.svelte:523-531` (search bar markup, no focus on mount) / `app/frontend/src/lib/actions/terminalSearchActions.js:4-22`
- Platform: all
- Symptom: After Ctrl+Shift+F the user types the search term; the characters appear at the shell prompt (or inside the running TUI) instead of the search field. Nothing indicates that the input is not focused.
- Repro:
  1. Click into terminal 1, press Ctrl+Shift+F.
  2. `document.activeElement` is `textarea.xterm-helper-textarea`; type `abc`.
  3. Observed: `WriteToTerminal` receives `"a"`, `"b"`, `"c"`; `.search-input` stays empty.
- Cause: `toggleTerminalSearch()` only flips `searchVisible` and focuses the *terminal* when closing (`terminalSearchActions.js:18-21`); the `<input class="search-input">` has no autofocus/`use:focus` action, and nothing calls `.focus()` after the bar renders.
- Fix sketch: focus the search input after `tick()` when `searchVisible` turns true (a small `use:autofocus` action as `MarkdownNotes.svelte:216` already has); keep the terminal focus on close.
- Evidence: `SplitPane.svelte:522-531`, `terminalSearchActions.js:4-22`. Live log: `active after Ctrl+Shift+F: textarea.xterm-helper-textarea`, `writes after typing abc: ["1:\"a\"","1:\"b\"","1:\"c\""]`.

### [P2] Ctrl+Shift+N / Ctrl+Shift+F are dead with CapsLock on and on non-Latin layouts (key swallowed, nothing happens)
- Area: `app/frontend/src/lib/actions/keyboardShortcuts.js:37` vs `:172` and `:179`
- Platform: all
- Symptom: With CapsLock active (or any state where `event.key` arrives lowercase for Ctrl+Shift+letter) Ctrl+Shift+N and Ctrl+Shift+F do nothing: the terminal swallows them (not sent to the shell) but Notes/Search do not toggle. On Cyrillic/Greek/Hebrew layouts all letter shortcuts (N F P W T M O U) are not recognised at all, because only `event.key` is compared and `event.code` is used for digits only.
- Repro (synthetic keydown on the xterm textarea, Ctrl+Shift held):
  1. `key:'N'` → `defaultPrevented: true`, notes panel toggles. `key:'n'` → `defaultPrevented: true`, notes panel does **not** toggle, no PTY write.
  2. `key:'F'` → search opens. `key:'f'` → prevented, search stays closed.
  3. `key:'Т'` (Cyrillic, `code:'KeyT'`) → not prevented, no shortcut.
- Cause: `isGlobalShortcut` accepts both cases (`'N','n','F','f'`, line 37) so xterm drops the key, while `createKeydownHandler` compares `event.key === 'N'` (172) and `=== 'F'` (179) only. Digits already use `event.code` (`DIGIT_CODE`, line 16); letters do not.
- Fix sketch: match letters on `event.code` (`KeyN`, `KeyF`, …) in both `isGlobalShortcut` and the handler, or at least use one shared predicate per shortcut so the "swallow" list and the "act" list cannot diverge.
- Evidence: `keyboardShortcuts.js:37,172,179`; prior-art check #7 (`docs/audit/01-prior-art.md`). Live log: `K1 n: {"prevented":true,"notes":1}` after `K1 N: {"notes":1}`; `K1 f: {"prevented":true,"search":0}`.

### [P2] Shortcut handler ignores `event.repeat` and `event.isComposing`: holding Ctrl+Shift+T opens one terminal per key repeat
- Area: `app/frontend/src/lib/actions/keyboardShortcuts.js:123` (`handleGlobalKeydown`, no guard)
- Platform: all (GTK auto-repeat starts after ~250 ms at ~30 Hz)
- Symptom: A slightly long press of Ctrl+Shift+T creates several terminals (each one spawns a PTY and a session-state save); long presses of Ctrl+Shift+N/F/P/W flicker their panels, Ctrl+Shift+M minimizes several panes in a row. Shortcuts also fire mid IME composition.
- Repro: dispatch three `keydown` events with `repeat: true`, `key:'T'`, Ctrl+Shift → `.terminal-wrapper` count goes 1 → 2 → 3 → 4; with `isComposing: true` → 5.
- Cause: no `if (event.repeat) return;` / `if (event.isComposing || event.keyCode === 229) return;` at the top of the handler; the xterm custom handler also swallows repeats, so the shell never sees them either.
- Fix sketch: early-return on `event.repeat` for all toggling/creating shortcuts (keep repeat for cycling if desired) and on `event.isComposing`.
- Evidence: `keyboardShortcuts.js:123-211`. Live log: `K1 repeat T x3: terms 2, 3, 4`, `isComposing T: terms 5`.

### [P2] Most modals cannot be closed with Escape and do not take focus when they open
- Area: `app/frontend/src/lib/modals/SSHProfileModal.svelte:146` (pattern repeated in AIPanelModal:81-91, AISettingsModal:28-38, AggDownloadModal:12-13, DotEnvViewerModal:122-123, FunctionCatalogModal:125-126, HostKeyModal:11, TemplatePromptModal:16-26, `workflows/WorkflowApprovalDialog.svelte:16-21`)
- Platform: all
- Symptom: Open the SSH profile manager from the sidebar "+" or the AI panel from the toolbar: focus stays on the button behind the modal (or on `body`); Escape does nothing; keystrokes do not reach the modal. Only clicking into the overlay background first, then Escape, closes it. In the modals whose dialog has `on:keydown|stopPropagation` (AIPanel, AISettings, Agg, DotEnv, FunctionCatalog, TemplatePrompt, WorkflowApproval) Escape pressed on a field or button *inside* the dialog is also swallowed.
- Repro:
  1. Click the "+" next to "SSH Hosts". `activeElement` = `button.sidebar-add-btn`. Press Escape → `.modal-overlay` count still 1.
  2. Toolbar "AI" → first menu item. `activeElement` = `body`; Escape → dialog still open. Focus the dialog's first button, Escape → still open. Focus `.modal-overlay` itself, Escape → closes.
  3. (Works) Ctrl+Shift+P: TemplatePicker focuses its input on mount and closes on Escape — the pattern to copy.
- Cause: the Escape listener sits on the overlay `div` with `tabindex="0"` and is only reached when that div is the focused element or in the bubbling path; nothing focuses the overlay/dialog on mount (`onMount(() => inputEl?.focus())` exists only in TemplatePicker:24, WorkflowPicker:21, TranscriptViewerModal:430). The inner `on:keydown|stopPropagation` prevents bubbling from fields to the overlay. `App.svelte`'s window Escape handler knows only the two pickers (`keyboardShortcuts.js:198-205`).
- Fix sketch: one shared modal wrapper that focuses the dialog (or first field) on mount, handles Escape on the dialog element (not the overlay), and does not stop keydown propagation; register all modal stores in a single Escape stack.
- Evidence: file:line list above; `TemplatePicker.svelte:24,46-51` for the working variant. Live log: `ssh modal after Escape: 1`, `AI modal after Escape on inner button: 1`, `AI modal after Escape on overlay: 0`.

### [P2] No focus trap and no focus restoration in modals (except TranscriptViewerModal)
- Area: `app/frontend/src/lib/modals/TemplatePicker.svelte:46-47` (representative; TranscriptViewerModal.svelte:369-445 is the only one that traps and restores)
- Platform: all
- Symptom: With a modal open, Tab leaves the dialog and walks the sidebar and the pane headers underneath (`aria-modal="true"` is declared but not enforced). After closing with Escape or ✕ the focus is on `body`: the user has to click back into the terminal before typing; keyboard-only users land at the start of the tab order.
- Repro:
  1. Ctrl+Shift+P, press Tab 12 times → `activeElement` = `div.sidebar-heading`, not inside `.template-picker`.
  2. Escape → `.template-picker` gone, `activeElement` = `body`.
  3. SSH modal open, Tab once from the "+" → `div.sidebar-heading` (behind the overlay).
- Cause: none of the ten `lib/modals/*` components (except TranscriptViewerModal) record the trigger element, trap Tab, or re-focus the terminal/trigger in their close path; `toggleTerminalSearch` and `closeContextMenu` likewise never call `terminal.focus()` on the Escape path.
- Fix sketch: lift the trap/restore code from `TranscriptViewerModal.svelte:369-445` into a reusable action and apply it to every modal; on close focus the previous `activeElement` or `focusTerminal(activeTerminalId)`.
- Evidence: `TranscriptViewerModal.svelte:369-407,430-445` (good), all other modals (no `focus()` call except on open). Live log above.

### [P2] Context menu is mouse-only: cannot be opened from the keyboard, takes no focus, arrow keys and typing go to the shell while it is open
- Area: `app/frontend/src/lib/SplitPane.svelte:444-495` (`role="menu"` markup) and `:137-156` (`openContextMenu`)
- Platform: all
- Symptom: With the menu visible, ArrowDown/Enter/letters are sent to the PTY (`\x1b[B`, `x`), Tab does not enter the menu, Escape does not close it (P1 above). Shift+F10 / the Menu key do not open it. A keyboard or screen-reader user has no access to copy-joined, tmux buffer, open-env, split, minimize or close from the pane.
- Repro:
  1. Click terminal 1, press Shift+F10 → `.ctx-menu` count 0 (Chromium normally synthesises `contextmenu` here; the keystroke is instead sent to the shell as `\x1b[21;2~`). Confidence medium for the keyboard-open part, high for the rest.
  2. Right-click the terminal → menu visible, `activeElement` still `textarea.xterm-helper-textarea`.
  3. ArrowDown, type `x` → `WriteToTerminal` gets `"\u001b[B"` and `"x"`. Tab → still in the textarea.
- Cause: `openContextMenu` only sets state; no element in `.ctx-menu` is focused, items are plain `<button role="menuitem">` without roving tabindex or key handling; the xterm textarea keeps focus and xterm stops propagation of every key.
- Fix sketch: on open, focus the first `menuitem`, add ArrowUp/Down/Home/End/Enter/Escape handling on the `role="menu"` element, restore `terminal.focus()` on close; accept `contextmenu` events without coordinates (place at cursor cell) so Shift+F10 works.
- Evidence: `SplitPane.svelte:137-156, 231-238, 437-495`. Live log: `K4 writes while ctx menu open: ["1:\"\\u001b[21;2~\"","1:\"\\u001b[B\"","1:\"x\""]`, `active after Tab: textarea… inside menu? false`.

### [P2] Split divider is a `role="slider"` with `tabindex="0"` but no keyboard handler and no name
- Area: `app/frontend/src/lib/SplitPane.svelte:585-594`
- Platform: all
- Symptom: Tab reaches the divider (it is a tab stop in every split), but ArrowLeft/Right/Up/Down do nothing; screen readers announce an unnamed slider "50". Pane ratios can only be changed with the mouse.
- Repro: split once, focus `.split-divider`, press ArrowRight twice → `aria-valuenow` stays `50`; `aria-label` is `null`.
- Cause: the element has `on:mousedown={startDrag}` only; `aria-valuenow` is bound to `localRatio` but no `on:keydown` updates it and dispatches `ratiochange`/`resize`.
- Fix sketch: add a keydown handler (arrows ±2 %, Shift ±10 %, Home/End) that updates `localRatio`, dispatches `ratiochange` and `resize`; add `aria-label` ("Resize panes").
- Evidence: `SplitPane.svelte:585-594`. Live log: `divider valuenow before/after: 50 50 name: null`.

### [P2] Focus is lost (lands on `body` or stays in the sidebar) after minimize, pane close and page switches
- Area: `app/frontend/src/lib/actions/terminalActions.js:92-110` (`finalizeTerminalRemoval`), `App.svelte:299-304` (`openPage('terminals')`)
- Platform: all
- Symptom: After clicking the pane's "–" (minimize) or "✕" (close) button, or after Settings → Terminal in the sidebar, nothing has focus; the next keystrokes go nowhere until the user clicks into a pane. The shortcut variants (Ctrl+Shift+M/O, Ctrl+Tab) do focus the next terminal, so behaviour is inconsistent between mouse and keyboard.
- Repro:
  1. Two panes; click "–" on the second → `activeElement` = `body`.
  2. Sidebar Settings, then sidebar Terminal → `activeElement` = `div.sidebar-heading.active-nav` (no terminal focused, although `reinitializeTerminals()` re-attached them).
  3. Close via "✕": `finalizeTerminalRemoval` sets `activeTerminalId` but never calls `focus()` (code reasoning, high; the mocked `CloseTerminal` resolves late so the live check was inconclusive).
- Cause: `terminalToBackground`/`finalizeTerminalRemoval` update stores only; `focusTerminal()` (`keyboardShortcuts.js:49`) is called from the shortcut paths but not from the button/dispatch paths or from `openPage`.
- Fix sketch: call `focusTerminal(get(activeTerminalId))` at the end of minimize/close handlers and after `reinitializeTerminals()` when returning to the terminals page.
- Evidence: `terminalActions.js:92-110, 628-636`, `App.svelte:299-304`, `keyboardShortcuts.js:49-58, 81-94`. Live log: `active after minimize by mouse: body`, `active back on terminals: div.sidebar-heading.active-nav`.

### [P2] Icon-only buttons are announced by their glyph ("–", "✕", "⏺", "◀", "+", "↻"); `title` does not help because content wins
- Area: `app/frontend/src/lib/SplitPane.svelte:401-411` (record/minimize/close), also `SSHProfileModal.svelte:150`, `AIPanelModal` `.modal-close-button`, `MarkdownNotes.svelte:245`, `AgentTranscriptPanel.svelte:442-443,463`, `Sidebar.svelte:144,267`, `TerminalsPage.svelte:97`
- Platform: all
- Symptom: Screen readers read the pane header as `button "BASH BASH 1 Open transcript ⏺ – ✕"` and the controls as "en dash button", "multiplication x button", "black circle button". The SSH and AI modal close buttons have no `title` at all. Sidebar headings include their disclosure glyphs in the name ("▸ Terminal ▸", "→ SSH Hosts + ▾").
- Repro: `ariaSnapshot()` of `.terminal-header` → `button "⏺"`, `button "–"`, `button "✕"`; only "Open transcript" (has `aria-label`) and the SVG split buttons (empty content → `title` used) get real names. Inventory of symbol-only names with the SSH and AI modals open: `button.header-btn.close-btn "✕"`, `button.modal-close-button "✕"`.
- Cause: accessible-name computation prefers text content over `title`; the buttons carry a glyph as content and rely on `title`.
- Fix sketch: add `aria-label={$t(...)}` to every glyph button (reuse the existing title strings), mark decorative glyph spans `aria-hidden="true"`.
- Evidence: file:line list above; live `ariaSnapshot` output in this audit's log.

### [P2] Right-click outside a pane shows the webview's default menu with "Reload"/"Back", which drops every terminal buffer
- Area: `app/main.go:82` (`EnableDefaultContextMenu: true`); only `SplitPane.svelte:421` handles `contextmenu`
- Platform: linux-webkitgtk, windows-webview2 (macOS WebKit shows Reload as well)
- Symptom: Right-click on the sidebar, pane header, notes panel, settings or any modal opens the browser menu (WebKitGTK: Back/Forward/Reload; WebView2: Back/Forward/Refresh/…). One click on Reload restarts the frontend: xterm scrollback and in-memory state are gone, PTYs are re-attached only via the session-restore path.
- Repro: code reasoning, confidence high (the native menu is not visible to Playwright). Prior-art check #1.
- Cause: the production build keeps the default menu for all surfaces; no `window` `contextmenu` guard and no `beforeunload` protection exist (`grep beforeunload` in `src/` is empty).
- Fix sketch: set `EnableDefaultContextMenu` false (or restrict it to text inputs via a `contextmenu` listener on `window` that `preventDefault()`s outside `input/textarea/[contenteditable]`), and add a `beforeunload` guard while terminals exist.
- Evidence: `app/main.go:82`, `SplitPane.svelte:421`, `docs/audit/01-prior-art.md` check 1.

### [P3] Global shortcuts are blocked while the search input or the restore-summary dialog has focus
- Area: `app/frontend/src/lib/SplitPane.svelte:523` (`.search-bar … on:keydown|stopPropagation`) and `:501` (`.restore-summary … on:keydown|stopPropagation`)
- Platform: all
- Symptom: With the cursor in the search field, Ctrl+Shift+F does not close the search, Ctrl+Shift+T does not open a terminal, Ctrl+Tab does not cycle panes; only Enter/Shift+Enter/Escape work. Same while the "restored transcript" overlay is focused.
- Repro: focus `.search-input`; Ctrl+Shift+T → terminal count stays 1; Ctrl+Tab → focus still in the input; Ctrl+Shift+F → `.search-bar` count stays 1. Escape in the field closes it and focuses the terminal (good).
- Cause: `stopPropagation` on the container swallows every keydown before the `svelte:window` handler in `App.svelte:507`.
- Fix sketch: drop the blanket `stopPropagation` and stop only the keys the bar consumes (Enter, Escape), or let `isGlobalShortcut(event)` pass through.
- Evidence: `SplitPane.svelte:501,523`. Live log: `Ctrl+Shift+T from inside search input -> terminals: 1`.

### [P3] Mimir takes Ctrl+Shift+Left/Right, Ctrl+−/=/0 and Ctrl+Tab away from the shell with no way to rebind
- Area: `app/frontend/src/lib/actions/keyboardShortcuts.js:24-38` (`isZoomShortcut`, `isGlobalShortcut`)
- Platform: all (CSI-u / modifyOtherKeys-aware TUIs affected most)
- Symptom: xterm would send `\x1b[1;6C/D` for Ctrl+Shift+Right/Left (word-wise shift-selection in micro, nano, helix and other TUIs), `\x1f` for Ctrl+− (readline `undo` on many keymaps, emacs negative argument) and Ctrl+Tab for tmux/vim users; all are intercepted. Hard-coded: there is no keymap setting, so a user who needs these in a TUI cannot opt out. Ctrl+Shift+C/Insert copy only when a selection exists, so the shell still gets them otherwise (good).
- Repro: focus terminal, press Ctrl+Right → PTY receives `"\u001b[1;5C"` (control). Ctrl+Shift+Right → no PTY write (swallowed). Ctrl+− → no PTY write, font size 13 px → 12 px.
- Cause: `isGlobalShortcut` returns true for these keys and the custom key handler cancels them (`terminalActions.js:149-151`); zoom also triggers with Shift held (no `!event.shiftKey` check).
- Fix sketch: offer a Settings option "pass Ctrl+Shift+Arrow / Ctrl+−/= to the shell" (the user prefers settings over fixed behaviour), or detect the kitty/CSI-u keyboard mode and let those through; keep Ctrl+Shift+1…9 as they are.
- Evidence: `keyboardShortcuts.js:24-38,130-142`, `terminalActions.js:136-151`. Live log: `Ctrl+Shift+Right writes: []`, `Ctrl+Right writes: ["1:\"\\u001b[1;5C\""]`, `Ctrl+- writes: [] font 13px -> 12px`.

### [P3] Nested interactive roles and dead tab stops in the pane header
- Area: `app/frontend/src/lib/SplitPane.svelte:325-336` (`terminal-wrapper` and `terminal-header` both `role="button" tabindex="0"`), `Sidebar.svelte:197-206,262-267,295-302` (headings `role="button"` containing `<button class="sidebar-add-btn">`)
- Platform: all
- Symptom: Each pane adds two Tab stops that do nothing on Enter (the header's Enter is ignored; the wrapper's Enter only re-activates the already active pane). Screen readers expose a button that contains buttons (the whole pane header is one button named by all its children). The sidebar "+" buttons live inside a button. Every Enter typed into the shell also bubbles to the wrapper's `handleWrapperKeydown` and dispatches `activate`.
- Repro: Tab order from the top: 11 sidebar stops, `select`, "+ New", "AI", then `terminal-wrapper`, `terminal-header`, `terminal-name`, the six header buttons, the xterm textarea. Focus `.terminal-header`, Enter → nothing. `ariaSnapshot` shows the nesting. Nested list: `button.sidebar-add-btn inside sidebar-heading`, `div.terminal-header inside terminal-wrapper`, `button.header-btn inside terminal-header`.
- Cause: `role="button"` was added to satisfy the a11y linter on click-handling divs rather than to describe the element; the header is a drag handle, not a button.
- Fix sketch: make the wrapper a `role="group"`/`region` with `aria-label={term.name}` and `tabindex="-1"`, remove `role/tabindex` from the header (keep `draggable`), convert the sidebar headings to real `<button>`s with the "+" rendered as a sibling.
- Evidence: `SplitPane.svelte:120-131,325-336,380-386`, `Sidebar.svelte:197-206`. Live tab-order log and `ariaSnapshot` output.

### [P3] Status indicators are colour-only: no text alternative for disconnect, SSH-files, agent and recording dots
- Area: `app/frontend/src/lib/SplitPane.svelte:341` (`.disconnect-dot`), `Sidebar.svelte:367` (`.ssh-files-dot`), `Sidebar.svelte:330` (`.sidebar-agent-dot`), `SplitPane.svelte:783-787` (`.agent-badge-dot`), `SplitPane.svelte:402` (`.record-btn` `class:recording`)
- Platform: all
- Symptom: Disconnected pane, "SSH terminal available for Files", agent working/permission/attention and "recording in progress" are conveyed only by a coloured (pulsing) dot or by the glyph's colour; nothing is announced, and colour-blind users get no second cue. The agent *row* in the sidebar also carries `agentStateText`, so only the dots themselves are affected there.
- Repro: code reasoning, confidence high (no agents/recordings in the mocked fixture; markup inspected).
- Cause: empty `<span>` elements with CSS background and no `aria-label`, `role="img"` or visually-hidden text; the record button's state is a class, not `aria-pressed`.
- Fix sketch: give each dot `role="img" aria-label={…}` (or a `.sr-only` span); set `aria-pressed={term.recording}` on the record button; use the badge's `title` text as `aria-label`.
- Evidence: file:line list above.

### [P3] No `prefers-reduced-motion` handling for the infinite pulse animations
- Area: `app/frontend/src/lib/SplitPane.svelte:783-788,1092-1095`, `app/frontend/src/styles/sidebar.css:262`
- Platform: all
- Symptom: Agent badges (0.9–1.4 s), sidebar agent dots and the recording glyph (1.5 s) pulse forever; panel/sidebar width transitions animate. Users who enabled "reduce motion" at OS level get no relief; the pulsing dots are exactly the vestibular trigger the media query exists for.
- Repro: `document.styleSheets` contain 0 rules under `@media (prefers-reduced-motion…)`; three `@keyframes` (`agent-pulse`, `pulse-record`, `sidebar-agent-pulse`) are declared with `infinite`.
- Cause: no reduced-motion block anywhere in `src/` (`grep prefers-reduced-motion` empty).
- Fix sketch: one global `@media (prefers-reduced-motion: reduce)` block that shortens animations/transitions to ~0 and makes the pulse states static (border or glyph change instead of opacity pulse).
- Evidence: `SplitPane.svelte:783-788,1092-1095`, `styles/sidebar.css:262`. Live log: `reduced-motion rules: 0`.

### [P3] Side panels (Notes, Agent) open without taking focus and cannot be closed with Escape; agent tabs have no arrow-key navigation
- Area: `app/frontend/src/lib/actions/keyboardShortcuts.js:172-177` (Ctrl+Shift+N), `MarkdownNotes.svelte:245` (close button only), `AgentTranscriptPanel.svelte:467-469` (`role="tablist"`/`tab` without roving tabindex)
- Platform: all
- Symptom: Ctrl+Shift+N opens Notes but focus stays in the terminal (the user has to Tab through the whole sidebar and pane headers to reach the editor); Escape does not close Notes or the Agent panel. The agent tablist exposes `role="tab"` + `aria-selected` but Left/Right do nothing, so screen-reader users hear a tablist that does not behave like one.
- Repro: Ctrl+Shift+N → `.notes-panel` count 1, `activeElement` = `textarea.xterm-helper-textarea`; Escape → `.notes-panel` still 1. Agent panel: code reasoning, confidence high.
- Cause: the toggle only flips `notesPanelOpen`; the Escape stack in `keyboardShortcuts.js:197-210` knows only the pickers and the search bars; the tab buttons are plain `on:click`.
- Fix sketch: focus the panel's first control when it opens via shortcut; add Notes/Agent panel to the Escape stack (close when focus is inside); add Arrow/Home/End handling on the tablist.
- Evidence: file:line list above. Live log: `notes panel active after Ctrl+Shift+N: textarea.xterm-helper-textarea`, `notes panel after Escape: 1`.

### [P3] Escape has no priority stack: one press closes pickers, every open search bar and the context menu together
- Area: `app/frontend/src/lib/actions/keyboardShortcuts.js:197-210` and `app/frontend/src/lib/SplitPane.svelte:234-238`
- Platform: all
- Symptom: When Escape does reach the window (focus outside xterm), the App handler closes the template picker *or* workflow picker, and otherwise *all* search bars in every pane at once, while SplitPane's independent window listener closes the context menu in the same keystroke. Expected: topmost overlay first (context menu → search → modal), one per press.
- Repro: code reasoning, confidence high (two `svelte:window on:keydown` listeners, both unconditional; `visibleSearches` loop at line 206-209). Live: with search + context menu open and focus in xterm neither closed (see P1).
- Cause: two uncoordinated window listeners; no notion of a focused overlay.
- Fix sketch: a small overlay stack store (push on open, pop on Escape) consumed by one Escape handler; close only the pane's own search bar when its input has focus.
- Evidence: `keyboardShortcuts.js:197-210`, `SplitPane.svelte:234-238,320`, `App.svelte:507`.

### [P3] Keyboard users must pass 11 sidebar stops before reaching the terminal; no skip link or focus shortcut to the active pane
- Area: `app/frontend/src/lib/Sidebar.svelte:144-192` (rail/headings), `views/TerminalsPage.svelte:62-64`
- Platform: all
- Symptom: Tab order from document start: sidebar toggle, 7 headings, 2 "+" buttons, shell `select`, "+ New", "AI", then per pane: wrapper, header, name, six buttons, xterm. Reaching the shell of the second pane takes ~25 Tabs; Ctrl+Tab cycles panes but there is no "focus active terminal" key when focus is elsewhere (e.g. after closing a modal).
- Repro: 14× Tab from a blurred document: `["button.sidebar-toggle","div.sidebar-heading.active-nav",…,"select","button.add-btn","button.ai-btn"]`.
- Cause: everything is a natural tab stop; headers/wrappers add extra stops (see nested-roles finding); no landmark/skip link.
- Fix sketch: `tabindex="-1"` on wrapper/header, a visually-hidden "Skip to terminal" link as first element, and make Ctrl+Tab (or Escape with nothing open) focus the active terminal when focus is outside the pane area.
- Evidence: live tab-order log; `Sidebar.svelte:144-192`, `SplitPane.svelte:325-336`.

---

## Not found / verified OK

- Ctrl+Shift+U on GTK is documented and Ctrl+Shift+O is the primary restore key (`keyboardShortcuts.js:157-164`) — correct handling of the GTK Unicode-input chord.
- Ctrl+Shift+1…9 use `event.code` (`DIGIT_CODE`) and therefore work on every layout; Ctrl+Shift+M/T/P/W/O/U accept both cases.
- Ctrl+Shift+C / Ctrl+Insert copy only when a selection exists and otherwise reach the shell (`terminalActions.js:137-148, 360-363`) — no conflict with `^C` in TUIs. Ctrl+Shift+V was not exercised (clipboard not available headless); paste is routed through `terminal.paste()` for bracketed paste (`terminalActions.js:347-356`).
- Escape works as expected when focus is on a non-xterm element (sidebar) and inside the search input (closes, refocuses terminal). Escape in TemplatePicker/WorkflowPicker/TranscriptViewerModal closes and these focus their input on open; TranscriptViewerModal also traps Tab and restores the trigger (`TranscriptViewerModal.svelte:369-445`).
- Shortcut variants restore focus correctly: Ctrl+Shift+M focuses the neighbouring pane, Ctrl+Shift+O focuses the restored one, Ctrl+Tab cycles.
- Sidebar headings carry `aria-expanded`; agent-panel pin uses `aria-pressed`; agent tabs carry `aria-selected`; the answer buttons (✓ Allow / ✕ Deny) have visible text; `SecretUnlockGate` is `role="dialog" aria-modal aria-label` with `role="alert"` for errors; the notes resize handle has `aria-label` (`TerminalsPage.svelte:147`).
- Zoom shortcuts are swallowed from the shell consistently (no stray `\x1f`), and `Ctrl+0` resets; font size changed 13 → 12 px live.
- Ctrl+Tab in WebView2 and Ctrl+Shift+Arrow/Ctrl+− in WebKitGTK were not verified against the native webviews (no `wails dev`); no accelerator-related Windows options are set in `app/main.go` (plain `wails.Run` options), so WebView2's own F5/Ctrl+R refresh is also unguarded — recorded for the platform domain.
- Only one running animation on an idle page (xterm cursor blink); the infinite pulses appear only with agents/recordings.
