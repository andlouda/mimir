# Findings — terminal-render

Domain: xterm.js 6 lifecycle inside Mimir — mount/unmount, fit and resize,
reflow, scrollback, search, Unicode width, cursor, selection/copy, DOM
renderer, theme propagation, disposal, hidden panes, page switch, per-chunk
store churn.

Method: key files read once (`actions/terminalActions.js`,
`terminals/xtermLifecycle.js`, `terminals/wheelScroll.js`,
`terminals/terminalLinks.js`, `terminals/multiRowLinks.js`,
`actions/terminalSearchActions.js`, `SplitPane.svelte`,
`stores/terminalStore.js`, `style.css`, `AppMainContent.svelte`,
`views/TerminalsPage.svelte`, `actions/keyboardShortcuts.js`, `index.html`,
`@xterm/addon-search` 0.16 source). Live reproduction with a throw-away
Playwright spec against the mocked build (Chromium; the mock stubs
`ResizeTerminal`, `WriteToTerminal`, `ClipboardSetText`, `EventsOnMultiple`
and `ResizeObserver` with counters). The spec was deleted afterwards.
Packages on this branch: `@xterm/xterm` 6.0.0, `addon-search` 0.16.0,
`addon-fit` 0.11.0; no unicode11 or webgl addon.

Counts: P1 ×1, P2 ×8, P3 ×5.

---

### [P1] Ctrl+Shift+F opens the search bar but leaves the keyboard in the shell
- Area: `app/frontend/src/lib/actions/terminalSearchActions.js:4` (`toggleTerminalSearch`)
- Platform: all
- Symptom: After Ctrl+Shift+F the search bar appears top-right, the cursor keeps blinking in the terminal and everything the user types is sent to the PTY. A query followed by Enter runs as a shell command.
- Repro: 1. Focus a terminal. 2. Press Ctrl+Shift+F — bar appears. 3. Type `nee`. Live: `document.activeElement` stays `TEXTAREA.xterm-helper-textarea`; `WriteToTerminal` received `n`, `e`, `e`; the search input stays empty.
- Cause: `toggleTerminalSearch` only flips `searchVisible` in the store; it calls `term.terminal.focus()` when *closing* (line 18–21) but never focuses `.search-input` when opening. The input in `SplitPane.svelte:524–531` has no `autofocus`/action either.
- Fix sketch: after the store update, `tick()` then focus the pane's `.search-input` (and select its text); on close keep the existing `terminal.focus()`.
- Evidence: `terminalSearchActions.js:8–21`; `SplitPane.svelte:522–535`; live log `focusAfterOpen: TEXTAREA.xterm-helper-textarea`, `typedWentToShell: "nee"`.

### [P2] Search highlights only one match: no highlight-all, no counter, no "no match" state
- Area: `app/frontend/src/lib/actions/terminalSearchActions.js:60` (`findNext(query)` without options)
- Platform: all
- Symptom: Typing a query selects the first hit; the other 59 hits look like plain text, the overview ruler stays empty, the bar shows only ▲ ▼ ✕ — no "3/60", and a query with zero hits looks identical to one with hits (selection just vanishes).
- Repro: 1. Emit 60 lines `L<n> alphabet`. 2. Ctrl+Shift+F, click the input, type `alpha`. Live: `.xterm-decoration` count 0, `.xterm-find-result-decoration` 0, one `.xterm-selection` block; type `zzz-not-there` → bar text `▲ ▼ ✕`, no selection, no class change. Screenshot `docs/audit/shots/terminal-render-search.png`.
- Cause: `findNext`/`findPrevious` are called with no `ISearchOptions`; addon 0.16 only builds decorations and fires `onDidChangeResults` when `decorations` is passed (`SearchAddon._highlightAllMatches`, `shouldUpdateHighlighting`). Nothing subscribes to `onDidChangeResults`, so no count can be shown.
- Fix sketch: pass `{ decorations: { matchBackground, activeMatchBackground, matchOverviewRuler, activeMatchColorOverviewRuler } }` on every call; subscribe to `searchAddon.onDidChangeResults` once per terminal and render `resultIndex+1/resultCount` (or "no match" + red border) in the bar.
- Evidence: `terminalSearchActions.js:40, 47, 60`; `terminalActions.js:127` (SearchAddon loaded with defaults); `SplitPane.svelte:522–535`.

### [P2] Emoji and ZWJ sequences occupy one cell (no unicode11 addon) — columns drift for programs that assume width 2
- Area: `app/frontend/src/lib/actions/terminalActions.js:113` (Terminal constructed without a Unicode provider)
- Platform: all
- Symptom: `🚀|X` renders the rocket over the `|`; a 👨‍👩‍👧 family draws as one smeared cell; everything after an emoji is one column left of where the program put it, so boxes drawn by Claude Code, lazygit, npm progress bars etc. get ragged right edges and the cursor lands one cell off after an emoji.
- Repro: 1. Emit `abc|X`, `🚀|X`, `漢字|X`, `👨‍👩‍👧|X`, `é|X`. Live glyph x-offset of `X` (cell ≈ 7.8 px): 31 / 16 / 39 / 16 / 16 px → emoji counted as width 1 (X in column 2), the ZWJ sequence as 1 cell total, CJK correctly as 2. Screenshot `docs/audit/shots/terminal-render-unicode.png`.
- Cause: xterm's built-in table is Unicode 6 widths; modern PTY programs (glibc wcwidth, Python, Rust `unicode-width`) treat emoji presentation as wide. `@xterm/addon-unicode11` is not in `package.json` and `terminal.unicode.activeVersion` is never set.
- Fix sketch: load `Unicode11Addon` and set `terminal.unicode.activeVersion = '11'` before `open()` (PR #48 reportedly does this); keep the DOM renderer, it respects the width table.
- Evidence: `terminalActions.js:113–129`; `package.json:24–27` (no unicode11); `docs/audit/01-prior-art.md` check 6.

### [P2] Ctrl+wheel zoom steps once per wheel event, so a touchpad flick jumps many sizes
- Area: `app/frontend/src/lib/SplitPane.svelte:83` (`handleTerminalWheel`, `if (event.ctrlKey)`)
- Platform: all (worst on touchpads / WebKitGTK pixel-delta wheels)
- Symptom: A short pinch or two-finger flick with Ctrl held moves the font from 13 px to the 28 px cap in one go; every terminal refits and the shell gets a burst of SIGWINCH.
- Repro: 1. Hover a terminal. 2. Dispatch 10 wheel events with `ctrlKey`, `deltaY: -4` (one touchpad gesture). Live: `mimir-terminal-font-size` 13 → 23, `.xterm-rows` font-size 23 px.
- Cause: `zoomTerminalFont(event.deltaY < 0 ? 1 : -1)` ignores `deltaY` magnitude; no accumulation unlike the scroll path, which already uses `consumeWheelEvent` with a per-terminal remainder.
- Fix sketch: accumulate `deltaY` per terminal (reuse `consumeWheelEvent` with a ~50 px threshold) and zoom one step per threshold crossing; or rate-limit to one step per 100 ms.
- Evidence: `SplitPane.svelte:83–88`; `stores/uiStore.js:44–46`; `terminals/wheelScroll.js:26–32` (the accumulation that the zoom path does not use).

### [P2] Wheel over the restored-transcript preview (and the search bar) scrolls the terminal, never the preview
- Area: `app/frontend/src/lib/SplitPane.svelte:419` (`on:wheel|capture|nonpassive` on `.terminal-container`)
- Platform: all
- Symptom: The "restored transcript" card is 140 px high with `overflow: auto`, but wheel scrolling over it moves the terminal rows behind it while the card's own content stays put; the only way to read the rest is dragging its scrollbar.
- Repro: 1. Start with a saved session (`GetLoadedSessionData` → one terminal) and a 40-line excerpt. 2. Emit 300 lines. 3. Wheel −240 over the card. Live: card `scrollTop` 0 → 0 (scrollHeight 688, clientHeight 138), terminal first row `L270` → `L257`.
- Cause: the capture-phase wheel handler on the container runs before the card and calls `event.preventDefault()` + `xterm.scrollLines()` for every target inside the container; the card's `on:mousedown|stopPropagation` etc. do not cover `wheel`. The search bar sits in the same container and has the same problem.
- Fix sketch: in `handleTerminalWheel` return early when `event.composedPath()` contains `.restore-summary`, `.search-bar` or `.ctx-menu`; or attach the wheel handler to `#terminal-<id>` instead of the container.
- Evidence: `SplitPane.svelte:416–421, 496–520, 835–847`; live log `RESTOREWHEEL`.

### [P2] Keyboard focus is not returned to a terminal after a page switch or after closing a pane
- Area: `app/frontend/src/lib/actions/terminalActions.js:391` (`reinitializeTerminals`) and `:93` (`finalizeTerminalRemoval`)
- Platform: all
- Symptom: Coming back from Settings, the sidebar heading keeps focus (its focus ring is visible), the terminal cursor is hollow, and typing does nothing until the user clicks into the pane. Same after closing the active pane with ✕: focus falls to `<body>`, the next active pane does not receive it.
- Repro: 1. Focus terminal 1. 2. Sidebar → Settings → Terminal. Live: `document.activeElement` = `DIV.sidebar-heading active-nav`. 3. Open a second terminal, click its ✕. Live after 900 ms: `activeElement` = `BODY`, one `.xterm` left, nothing focused. (Ctrl+Shift+O after a minimize does focus, via `focusTerminal` in `keyboardShortcuts.js`.)
- Cause: `reinitializeTerminals` only re-attaches and refits; `finalizeTerminalRemoval` picks `activeTerminalId` but never calls `terminal.focus()`; `openPage('terminals')` (`App.svelte:300–306`) has no focus step either. Only `createTerminalInstance` and the restore shortcut focus.
- Fix sketch: at the end of `reinitializeTerminals` and `finalizeTerminalRemoval`, focus the xterm of `activeTerminalId` when the terminals page is current; keep `focusTerminal` as the single helper.
- Evidence: `terminalActions.js:93–111, 391–406`; `App.svelte:300–306`; `keyboardShortcuts.js` `focusTerminal` (used only by the shortcut paths); live logs `PAGESWITCH.after.active`, `DISPOSAL.afterClose.active`.

### [P2] Terminal font comes from fonts.googleapis.com at runtime: offline fallback, online late swap after xterm measured the cell
- Area: `app/frontend/index.html:9` (Google Fonts `<link>`; CSP line 6 allows it)
- Platform: all (worst on WebKitGTK / first launch)
- Symptom: Without network (or on a corporate proxy) terminals render in Fira Code / Cascadia / whatever `monospace` resolves to — a different look per machine. With network, the page paints with the fallback (`display=swap`), xterm `open()`s and measures the cell with that font, then JetBrains Mono swaps in: glyph advance and measured cell differ until the next fit, so selection boxes and the cursor sit slightly off the glyphs. Each launch also contacts Google.
- Repro: code reasoning, confidence medium. Live run had fast network: `document.fonts` listed JetBrains Mono `loaded` before `open()`, cell 7.826 px vs glyph 7.84 px, so the swap case could not be forced in the mock (route delay did not beat the preconnect). Offline case: `font-src https://fonts.gstatic.com` only — no local `@font-face` exists in `style.css`.
- Cause: no bundled font; `fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace"` (`terminalActions.js:116`); nothing listens to `document.fonts.ready` to re-measure (`terminal.options.fontFamily` re-assignment or `fit()`).
- Fix sketch: ship the font locally (PR #48 does) and drop the Google link + CSP entries; additionally `document.fonts.ready.then(() => handleResize())` to re-measure after any late load.
- Evidence: `index.html:6–9`; `terminalActions.js:116`; `style.css` (no `@font-face`); `00-surface-map.md` note on PR #48.

### [P2] Divider drag / window resize reflows the full scrollback on every frame while the PTY waits 300 ms
- Area: `app/frontend/src/lib/terminals/xtermLifecycle.js:86` (`observeTerminalResize` → `safelyFitAndResizeTerminal` per animation frame)
- Platform: all
- Symptom: With a long scrollback a divider drag stutters (each frame re-wraps every buffer line), while the program inside only learns the final size 300 ms after the drag ends — so during the drag the shell/TUI is drawn at the old width inside a pane that already reflowed its history.
- Repro: 1. Emit 20 000 lines of 130 columns. 2. Change the viewport width six times within 290 ms (1100 → 1000 → 900 → 1000 → 1100 → 1280). Live: `ResizeTerminal` IPC once (debounce works), but two `longtask` entries of 65 ms and 52 ms during the burst from `fitAddon.fit()` → `terminal.resize()` reflow. Scrollback is 100 000 (`terminalActions.js:120`), so the per-frame cost at a full buffer is ~5× that — a 250–300 ms frame per width change.
- Cause: the ResizeObserver coalesces to one `fit()` per frame but does not debounce it; every `fit()` with a changed column count runs xterm's reflow over the whole buffer. The PTY debounce (`RESIZE_SETTLE_MS = 300`, `scheduleResize`) only delays the IPC, not the local reflow.
- Fix sketch: during an active divider drag (`isDragging` in `SplitPane.svelte`) skip `fit()` and fit once on mouseup (the `resize` event already exists); or debounce `fit()` itself to ~50 ms; consider lowering the default scrollback or making it a setting.
- Evidence: `xtermLifecycle.js:19–58, 86–94`; `SplitPane.svelte:26–49` (drag dispatches `ratiochange` each move and `resize` on mouseup); `terminalActions.js:120`; live log `RESIZE`.

### [P2] Context menu overflows the window bottom when link items are present — Minimize/Close unreachable
- Area: `app/frontend/src/lib/SplitPane.svelte:136` (`CTX_MENU_HEIGHT = 260`)
- Platform: all
- Symptom: Right-clicking a URL in the bottom rows of a pane opens a menu whose last two entries ("Minimize", "Close") are below the window edge; the menu cannot be scrolled.
- Repro: 1. Emit 40 filler lines then `see https://example.com/some/path`. 2. Hover the URL, right-click. Live (720 px window): menu top 460, height 326, bottom 786 → 66 px off-screen; without the link block the menu is 261 px and fits by 1 px. Screenshot `docs/audit/shots/terminal-render-ctxmenu.png`.
- Cause: the clamp `Math.min(event.clientY, window.innerHeight - CTX_MENU_HEIGHT)` uses a fixed 260 px, but the menu has conditional items (open/copy link +2, copy joined +1, tmux buffer +1, Shift hint) and the rendered base height is already 261 px.
- Fix sketch: position after mount — measure `.ctx-menu` `offsetHeight` in `tick()` and clamp with the real size, or flip the menu upwards when `clientY + height > innerHeight`.
- Evidence: `SplitPane.svelte:135–157, 437–494`; live log `CTXLINK`.

### [P3] Escape inside the terminal sends ESC to the shell and leaves the search bar open
- Area: `app/frontend/src/lib/actions/keyboardShortcuts.js:197` (Escape branch of the window keydown handler)
- Platform: all
- Symptom: With the search bar open and the keyboard in the terminal, Escape does not dismiss it (the key reaches the shell as `\x1b`); only Escape inside the input or the ✕ button closes it.
- Repro: 1. Ctrl+Shift+F, click into the terminal. 2. Press Escape. Live: `WriteToTerminal` got `"\u001b"`, `.search-bar` still visible.
- Cause: xterm's `_keyDown` calls `cancel(event, true)` — `preventDefault` **and** `stopPropagation` — for every key it handles, so the window listener never sees Escape. The comment in `terminalActions.js:131–133` ("xterm does not stop propagation") only holds for keys the custom handler rejects (`isGlobalShortcut`), which Escape is not.
- Fix sketch: in `attachCustomKeyEventHandler`, when `Escape` is pressed and the terminal's `searchVisible` is set, close the search and return `false`; or add `Escape` to the global-shortcut list only while a search is open.
- Evidence: `terminalActions.js:137–152`; `keyboardShortcuts.js:197–210`; xterm 6 `Terminal.cancel` (`xterm.js` lib).

### [P3] Ctrl+Shift+F inside the search input does not toggle the bar closed
- Area: `app/frontend/src/lib/SplitPane.svelte:523` (`.search-bar on:keydown|stopPropagation`)
- Platform: all
- Symptom: The shortcut that opened the bar does nothing while the caret is in the input; the user has to press Escape or click ✕. Every other global shortcut (Ctrl+Shift+T, N, P, W, Tab cycling, zoom) is also dead while the input has focus.
- Repro: 1. Ctrl+Shift+F, click into the input. 2. Ctrl+Shift+F again. Live: `.search-bar` still visible.
- Cause: the `stopPropagation` modifier on the bar swallows every keydown before it reaches the `<svelte:window>` handler.
- Fix sketch: drop `|stopPropagation`; `handleSearchKeydown` already `preventDefault`s Enter/Escape, and xterm cannot receive the events anyway because the input is the target.
- Evidence: `SplitPane.svelte:51–62, 522–535`; `keyboardShortcuts.js:179–183`.

### [P3] Every PTY chunk triggers an `AppendTerminalTranscript` IPC call and a full store clone — IPC is the real cost, the store churn is cheap
- Area: `app/frontend/src/lib/actions/terminalActions.js:234` (`terminal-output-<id>` handler)
- Platform: all (IPC cost highest on WebView2/WebKitGTK bridges)
- Symptom: During fast output (`yes`, build logs, agent streaming) the frontend sends one JSON bridge message per chunk to Go in addition to the PTY write; in the mock 2 000 chunks produced 2 000 `AppendTerminalTranscript` calls.
- Repro: 1. Emit 2 000 chunks of 90 bytes to terminal 1. Live: 11.4 ms total for the handler loop with one pane, 7–9 ms with four panes (store clone + 12 kB slice ≈ 5 µs/chunk, Svelte 5 batches the re-render into one 90 ms frame); `AppendTerminalTranscript` +2 000. Prior-art check 3 ("every `$:` consumer re-runs") is therefore not a measurable problem on Svelte 5; the IPC fan-out is.
- Cause: `appendTerminalTranscript(resumeId, data)` is called per chunk with no batching; the store update per chunk is harmless but also unnecessary (the 12 kB `outputBuffer` is only read by the AI panel).
- Fix sketch: buffer transcript appends per terminal and flush every ~250 ms or 16 kB; move `outputBuffer` out of the store (plain ring buffer read on demand).
- Evidence: `terminalActions.js:234–243`; `transcript/transcriptApi.js:117–123`; live log `CHURN`.

### [P3] tmux "invisible" wheel scrolling is asymmetric: scrolling up moves one key (3 lines) further than scrolling down
- Area: `app/frontend/src/lib/terminals/wheelScroll.js:47` (`tmuxScrollKeys`)
- Platform: linux-webkitgtk, macos-webkit (tmux-capable shells)
- Symptom: In a tmux pane with the invisible integration, wheel up by one notch then wheel down by one notch does not return to the same place — the view ends 3 lines above where it started; repeated up/down jitter drifts the history upward.
- Repro: code reasoning, confidence medium (needs a real tmux). `tmuxScrollKeys(-6)` → `S-PPage` ×3, `tmuxScrollKeys(6)` → `S-NPage` ×2; the extra up key "enters copy-mode" but, once in copy-mode, scrolls another 3 lines.
- Cause: `if (n < 0) return UP.repeat(steps + 1)` — the copy-mode-entry key is also a scroll key.
- Fix sketch: track per terminal whether copy-mode was already entered (first up-scroll sends the extra key, later ones do not), or bind a dedicated non-scrolling copy-mode entry key in the tmux config Mimir installs.
- Evidence: `wheelScroll.js:38–53`; `SplitPane.svelte:89–105`.

### [P3] `wireTerminalDom` paste listener is dead code — xterm already stops the event on its textarea
- Area: `app/frontend/src/lib/actions/terminalActions.js:348` (`handlePaste` on `term.terminal.element`)
- Platform: all
- Symptom: None visible (no double paste — verified live: one `WriteToTerminal` call with `PASTE-X\rline2` for a paste event on the textarea). The comment claims this listener routes paste through xterm for bracketed paste, but xterm's own `paste` listener on the textarea calls `stopPropagation` and `handlePasteEvent` first, so the element-level listener never runs; the real bracketed-paste behaviour comes from xterm. If a future xterm version stops swallowing the event, the paste would be sent twice.
- Repro: code reasoning, confidence high (xterm 6 `Clipboard.handlePasteEvent` → `ev.stopPropagation()`; listeners registered on both `textarea` and `element`).
- Cause: redundant listener added on the assumption that xterm leaves the browser default in place.
- Fix sketch: remove the listener and the comment, or keep it only as `if (event.defaultPrevented) return`.
- Evidence: `terminalActions.js:347–358`; `node_modules/@xterm/xterm/lib/xterm.js` (`addDisposableListener(this.textarea,"paste",…)` and `(this.element,"paste",…)`); live log `COPYPASTE.pasteWrites`.

---

## Not found / verified OK

- Disposal: closing a pane unsubscribes `terminal-output-<id>` and `terminal-closed-<id>` (on 1 / off 1 each), removes the `.xterm` node, drops the ResizeObserver, and output emitted for the closed id afterwards does not reach `AppendTerminalTranscript`. No listener/observer leak found in `cleanupTerminalResources`.
- Page switch keeps the xterm DOM node (same element re-parented by `safelyAttachTerminal`), the buffer, and the scroll position (viewport first row `pre 136` before and after Settings → Terminal); output written while away lands in the buffer; no extra `ResizeTerminal` IPC when the size is unchanged; no console errors.
- Minimized pane: output written while minimized is present after restore (`KEEP-ME`, `WHILE-MIN`); Ctrl+Shift+O refocuses the terminal and the cursor is `xterm-cursor-blink xterm-cursor-bar`.
- Search is effectively incremental in addon 0.16: extending `alpha` → `alphab` kept the same selection (the addon restarts from the selection start when the term changed), so prior-art check 4's "each keystroke jumps to the next match" does not reproduce. Escape in the input and the ✕ button close the bar and return focus to the terminal.
- Resize debounce: six viewport changes within 290 ms produced exactly one `ResizeTerminal` IPC; first size of a terminal is sent immediately; `forgetTerminalResize` clears timers on close.
- Copy: Ctrl+Shift+C with a mouse selection → `ClipboardSetText("hello wo")`; without a selection it neither copies nor sends `^C` to the PTY. Context menu Copy / Paste / Select all work; OSC 52 write `\e]52;c;SGVsbG8=\a` reaches the clipboard (write-only provider, reads return '').
- Theme propagation: `--bg-void` `#0c0e14` = `XTERM_THEME.background` = `.xterm-viewport` background = `body` background; no light theme exists on this branch, so there is no out-of-sync theme path. (Wails window `BackgroundColour` mismatch belongs to theme-dpi.)
- CJK width (漢字) is correct (2 cells); precomposed `é` is 1 cell.
- Alternate-screen wheel without tmux (vim, less) falls through to xterm, which converts it to arrow keys; with a mouse-tracking program the wheel is forwarded as mouse events.
- `fitAddon.fit()` on a detached container is a no-op (fit's NaN guard), so the stale `term` closure in the ResizeObserver does not resize minimized terminals.
- Enter/Space keydown bubbling from the shell to `.terminal-wrapper` re-dispatches `activate`, but `activeTerminalId = id` with the same value does not notify subscribers; no churn.
