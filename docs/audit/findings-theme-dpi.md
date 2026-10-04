# Findings — theme-dpi

Scope: dark-only theme (`src/styles/base.css` tokens, `src/style.css`), `prefers-color-scheme`
and `color-scheme`, window background in `app/main.go` vs CSS/xterm, contrast of the text
tokens against the five surfaces (computed WCAG ratios), hardcoded colours vs tokens
(status palette `#e3b341 #7ee787 #ff7b72 #63b3ed #d2a8ff` is agreed), high-DPI 125–200 %,
font fallback (CDN fonts, no bundled font on this branch), hover/active/disabled/focus-visible
per button class, scrollbars, colour emoji vs monochrome glyphs.

Method: contrast ratios from a node script over the `:root` tokens; greps for hex literals
(18 files, ~60 distinct values) and for `font-family`/`outline`/`:disabled`/`:focus-visible`
rules; three throw-away Playwright passes (deleted) with the mock fixture at 1280×800 @1x,
1024×700 @2x, plus a CDP `setDeviceMetricsOverride` to switch the DPR 1 → 2 → 1.25 without
changing the CSS viewport, `emulateMedia({colorScheme:'light'})`, computed styles of the
button classes, `document.fonts` state and the font request log. What depends on WebKitGTK /
WebView2 chrome (native popups, launch flash) is code reasoning.

Contrast table (WCAG, text token on surface): primary 12.5 / 11.9 / 11.3 / 10.4 / 9.4;
secondary 5.17 / 4.92 / 4.69 / 4.32 / 3.89; muted 2.89 / 2.75 / 2.62 / 2.41 / 2.17; accent
8.45…6.35; error `#f47067` 6.76…5.08; `#ff7b72` 7.65…5.75; `#f85149` 5.75…4.32
(surfaces in order void / deep / surface / raised / overlay). `--border-subtle` 1.11:1,
`--border-dim` 1.27:1, `--accent-glow` hover 1.20:1, white-6 % hover 1.16:1.

### [P2] 1 px white line down the right edge of every terminal pane (xterm overview-ruler border)
- Area: `app/frontend/src/lib/actions/terminalActions.js:26` (`XTERM_THEME`)
- Platform: all
- Symptom: Each pane shows a full-height, pure-white 1 px vertical line 8 px from its right edge, even with an empty terminal. It reads as a stray border and matches no token.
- Repro: 1. Open the app (mock fixture, 1280×800 @1x). 2. Sample pixel column 1272 at y = 100/300/600: `(255,255,255)` while both neighbours are `--bg-void`. 3. `.xterm-decoration-overview-ruler` is at x = 1272, 8×700 px; `.slider` is 0×0 (no scrollback), so the line is not the slider.
- Cause: `overviewRuler: { width: 8 }` (`terminalActions.js:121`) enables xterm 6's overview ruler; its renderer paints `fillRect(0,0,1,height)` with `theme.overviewRulerBorder`, and `XTERM_THEME` sets the slider colours but not `overviewRulerBorder`, so xterm's default (white at runtime; `typings/xterm.d.ts:379` still documents black) is used.
- Fix sketch: Add `overviewRulerBorder: '#0c0e14'` (or `rgba(99,179,237,0.15)` to match `--border-dim`) to `XTERM_THEME`.
- Evidence: `terminalActions.js:26-32,121`; `node_modules/@xterm/xterm/lib/xterm.js` (`colors.overviewRulerBorder.css … fillRect(0,0,1,…)`); `docs/audit/shots/theme-dpi-terminals-1280-dsf1.png`; `docs/audit/shots/theme-dpi-ruler-border-zoom.png`

### [P2] Terminal cell metrics are measured with the fallback font; `lineHeight: 1.35` is not applied until the first zoom or DPR change
- Area: `app/frontend/src/lib/actions/terminalActions.js:112-122`
- Platform: all
- Symptom: Rows are 20 px tall after start (35 rows in a 719 px container) although JetBrains Mono 13 px × 1.35 gives ~22.5–23 px; text sits tighter than designed. The first Ctrl+/−, a DPR change or a font-size change re-measures and the row count drops by ~13 % (35 → 30 rows at the same height), so the layout "jumps" on the first zoom step and the PTY is resized at that moment.
- Repro: 1. Open the app at 1280×800 @1x; wait 800 ms. 2. `.xterm-rows > div` height = 20.00, `.xterm-screen` 1033×700, `document.fonts.check("13px 'JetBrains Mono'")` = true. 3. Force a re-measure (CDP DPR 2, or zoom): row height = 22.50, same 35 rows → screen 788 px. Confidence that the 20 px comes from the fallback `monospace` (DejaVu/Liberation Mono: 14.8 px × 1.35 ≈ 20): medium-high; the second measurement is what the loaded font yields.
- Cause: `createTerminalInstance` opens the terminal immediately; `index.html:9` loads JetBrains Mono from Google Fonts with `display=swap`, so xterm's first char measurement runs against the fallback font, and xterm never re-measures on `document.fonts` events. Nothing in `src/` awaits `document.fonts.ready`.
- Fix sketch: Before opening the first terminal `await document.fonts.load("13px 'JetBrains Mono'")` (bounded by a short timeout); when `fonts.ready` resolves later, re-apply `terminal.options.fontFamily` to force a re-measure and refit. PR #48's bundled font narrows but does not remove the race.
- Evidence: `terminalActions.js:112-122`; `index.html:9`; `xtermLifecycle.js:86-92` (refits only on ResizeObserver); live numbers above

### [P2] DPR change without a window resize leaves the terminal overflowing its container (rows hidden, PTY size stale)
- Area: `app/frontend/src/lib/terminals/xtermLifecycle.js:86`
- Platform: all (monitor move 100 % ↔ 150/200 %, RDP/VDI reconnect, Windows display-scale change)
- Symptom: After the display scale changes, xterm re-measures and the row height grows (20 → 22.5 px) but the row count stays at 35, so `.xterm-screen` becomes 788 px inside a 719 px `.terminal-container` (overflow hidden): the last 3 rows, including the prompt line, are invisible until the user resizes the window or splits. The shell still believes it has 35 rows.
- Repro: 1. Open the app at 1280×800 @1x. 2. CDP `Emulation.setDeviceMetricsOverride({width:1280,height:800,deviceScaleFactor:2})`. 3. After 700 ms: `.terminal-container` 1060×719.3, `.xterm-screen` 1056×788, rows 35, `ResizeTerminal` calls 0. 4. Shrink the width by 1 px → refit: 29 rows at 22.52 px.
- Cause: xterm 6 has its own `matchMedia('(resolution: Xdppx)')` listener and re-measures the cell size, but Mimir only refits from a `ResizeObserver` (`xtermLifecycle.js:86`) and `window resize` (`App.svelte:428`); neither fires when only `devicePixelRatio` changes.
- Fix sketch: Register a `matchMedia(`(resolution: ${devicePixelRatio}dppx)`)` change listener (re-armed after each change) that calls `handleResize()`.
- Evidence: `xtermLifecycle.js:79-100`; `terminalActions.js:407-415`; `App.svelte:428`; live numbers above

### [P2] Window background `#1b2636` does not match `--bg-void` `#0c0e14`: lighter navy flash at launch and on resize
- Area: `app/main.go:59`
- Platform: linux-webkitgtk, windows-webview2
- Symptom: The native window paints a noticeably lighter blue-grey (`rgb(27,38,54)`, Δ = +15/+24/+34 per channel, 1.27:1 against the page) before the webview's first frame, and the same colour shows in the freshly exposed strip while the window is enlarged (WebKitGTK and WebView2 paint the native background until the web content catches up). On the dark page this reads as a flicker.
- Repro: code reasoning, confidence high (Wails #2852 / #1655 class; 1228×922 window at launch).
- Cause: `BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1}` (`main.go:59`) is a leftover from an older palette; the frontend uses `#0c0e14` in `style.css:12`, `base.css:11` and `XTERM_THEME.background`.
- Fix sketch: Set `BackgroundColour` to `RGBA{12,14,20,1}` and keep it in step with `--bg-void` (one constant, referenced from a comment in `base.css`).
- Evidence: `main.go:59`; `style.css:12`; `base.css:11`; `terminalActions.js:27`

### [P2] UI and terminal fonts are fetched from Google Fonts on every launch; offline there is no designed fallback, and the CSP would block a bundled font
- Area: `app/frontend/index.html:6-9`
- Platform: all
- Symptom: A desktop audit tool contacts `fonts.googleapis.com` / `fonts.gstatic.com` on every start (two requests observed). Offline or on a locked-down VDI the UI renders in `sans-serif` (DejaVu Sans on Linux, Arial on Windows; no `Segoe UI`/`system-ui` in the stack) and the terminal in the platform `monospace`, with a visible font swap (`display=swap`) when the network is slow. Every text width in the sidebar/headers changes between the two states.
- Repro: 1. Load with network: responses `200 fonts.googleapis.com/css2?…`, `200 …/dmsans/…woff2`, `200 …/jetbrainsmono/…` (Playwright request log). 2. Offline: code reasoning, confidence high. `font-src https://fonts.gstatic.com` is the only allowed font origin; `'self'` is missing, so a bundled `@font-face` (PR #48) is refused by the CSP unless `index.html:6` changes with it.
- Cause: `index.html:9` `<link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono…&family=DM+Sans…&display=swap">`; `style.css:13` `'DM Sans', -apple-system, BlinkMacSystemFont, sans-serif`; `base.css:31-32` `--font-mono`/`--font-sans` without Windows/Linux system fonts.
- Fix sketch: Bundle both families (PR #48 does the mono one) and add `'self'` to `font-src`; drop the Google link and preconnects; extend the stacks with `system-ui, 'Segoe UI', 'Noto Sans'` and `'Cascadia Mono', Consolas, 'DejaVu Sans Mono'`.
- Evidence: `index.html:6,8-9`; `style.css:13`; `base.css:31-32`; `terminalActions.js:116`

### [P2] `--text-muted` (2.9:1) and `--text-secondary` on raised surfaces (3.9–4.3:1) fail WCAG AA; nav icons at opacity .7 land at 3.1:1
- Area: `app/frontend/src/styles/base.css:18-20`
- Platform: all
- Symptom: Settings notes, placeholders, the "No profiles / No agents detected" hints, header buttons, the sidebar toggle, `.ai-hub-link` labels and the "AI: … active" status are rendered in `#545d68`, which is hard to read on any of the five surfaces (2.89:1 on `--bg-void`, 2.17:1 on `--bg-overlay`). Secondary text (`#7d8590`) passes only on the two darkest surfaces (5.17 / 4.92) and fails on cards and overlays (4.69 / 4.32 / 3.89). Nav-rail glyphs are drawn at `opacity: 0.7` of secondary → 3.09:1.
- Repro: computed (WCAG relative luminance), confidence high. Live: `.sidebar-toggle` colour `rgb(84,93,104)` on `rgb(12,14,20)`; `.nav-icon` opacity .7/.8 (`sidebar.css:189,81`).
- Cause: `--text-muted: #545d68` and `--text-secondary: #7d8590` were chosen for `#0c0e14` but are used unchanged on `--bg-raised`/`--bg-overlay`; `.nav-icon { opacity: 0.7 }` and `.collapsed-icon .nav-icon { opacity: 0.8 }` stack a second dimming on top.
- Fix sketch: Raise `--text-muted` to ≈`#6e7a88` (4.5:1 on void) and `--text-secondary` to ≈`#8b949e` (which four rules already hardcode), or add `--text-muted-on-raised`; drop the icon opacity and use colour instead.
- Evidence: `base.css:18-20`; `sidebar.css:41-47,81,187-193`; `ai-hub.css:113-116,138-143`; contrast table above

### [P2] Recording player uses a different terminal theme (Tokyo Night) and font list than live terminals
- Area: `app/frontend/src/lib/RecordingPlayer.svelte:49-60`
- Platform: all
- Symptom: Opening a recording shows a terminal with a `#1a1b26` background, `#a9b1d6` text and a different ANSI palette (`#f7768e` red, `#7aa2f7` blue, `#9ece6a` green) on Mimir's `#0c0e14` page, with a different font order (`Cascadia Code` before `Fira Code`, plus `Menlo`) and no `lineHeight`. The same output looks different in the recording than in the pane it was recorded from; the page reads as a second app.
- Repro: code reasoning, confidence high (theme literal in the component).
- Cause: `RecordingPlayer.svelte:46-60` instantiates its own `Terminal` with an inline Tokyo Night theme instead of `XTERM_THEME`; `recording-player.css` and `history-dashboard.css` carry the matching leftover surfaces (`#1a1e2e`, `#292e42`, `#565f89`).
- Fix sketch: Export `XTERM_THEME` and the font/lineHeight options from `terminalActions.js` (or a `terminals/theme.js`) and reuse them in the player; replace the Tokyo surfaces in the two CSS files with tokens.
- Evidence: `RecordingPlayer.svelte:49-60`; `recording-player.css` (9× `#a9b1d6`, 10× `#7aa2f7`); `overlays.css:749-782` (Tokyo fallbacks on defined vars); `SplitPane.svelte:1082` (`#565f89`)

### [P3] Colour emoji in Settings cards, file browser and .env viewer break the monochrome icon language
- Area: `app/frontend/src/lib/views/SettingsView.svelte:115`
- Platform: all (glyph set varies per OS emoji font)
- Symptom: Five Settings cards use colour emoji (🤖 `U+1F916`, 📝, 📂, 📜, 🎬) that ignore `color: var(--accent)` and render as 20 px multicolour bitmaps next to the accent-blue monochrome glyphs (A, ≡, ♪, ✱, ⚛, ✎, ⇧) at 13–14 px. The file browser uses 📁/📄 and the .env viewer 🙈/👁. The sidebar was deliberately moved to monochrome glyphs (commit `385f65d`), so Settings now contradicts it; on Windows the Segoe UI Emoji bitmaps are larger still.
- Repro: 1. Open Settings at 1024×700 @2x. 2. `.ai-hub-icon` widths: 20 px for the five emoji vs 8–14 px for the glyphs; the 🤖 renders red/white while its computed `color` is `rgb(99,179,237)`.
- Cause: `SettingsView.svelte:115,174,183,192,201` (`&#x1F916; &#x1F4DD; &#x1F4C2; &#x1F4DC; &#x1F3AC;`); `FileBrowser.svelte:230`; `DotEnvViewerModal.svelte:182`.
- Fix sketch: Replace with monochrome glyphs or inline SVG (✱ agents, ✎ notes, ⌂ files, ≡ transcripts, ▶ recordings) and set `font-variant-emoji: text` / append `U+FE0E` where a text presentation exists.
- Evidence: `SettingsView.svelte:89-210`; `sidebar.css:187-193`; `docs/audit/shots/theme-dpi-settings-emoji-dsf2.png`

### [P3] Three reds, three secondary greys and two stray accent blues: hardcoded colours drift from the tokens
- Area: `app/frontend/src/lib/AgentTranscriptPanel.svelte:784`
- Platform: all
- Symptom: Error/deny states use `#f47067` (`--error`, context menu, disconnect overlay), `#ff7b72` (agreed status red: sidebar permission dot, agent badge, deny buttons, host errors) and `#f85149` (agent panel errors, delete ops, task priority, failed exits) side by side in the same panel. Drop indicators and the .env viewer fall back to `#6ea8ff`/`#8ab4ff` because `--accent-color` is never defined (the token is `--accent`). `#8b949e` appears four times next to `--text-secondary #7d8590`. History dashboard and the pane context menu use their own surfaces (`#1a1e2e`, `#2a2f3e`, `#2b3140`) instead of `--bg-raised`/`--bg-overlay`.
- Repro: code reasoning, confidence high (`grep -rhoE "#[0-9a-f]{3,8}"` → 60 distinct values in 18 files; the tokens cover 20).
- Cause: `base.css:21` defines `--error: #f47067` while the agreed palette red is `#ff7b72`; `SplitPane.svelte:694,696` and `DotEnvViewerModal.svelte:266,288` reference `var(--accent-color, …)`; `history-dashboard.css:15,35,47,57-58,91-92,111-112` and `SplitPane.svelte:920,979` hardcode surfaces.
- Fix sketch: Add `--status-red/--status-green/--status-yellow/--status-magenta` tokens (the agreed five) and point `--error` at `#ff7b72`; rename `--accent-color` → `--accent`; replace the surface literals in history-dashboard/SplitPane with tokens.
- Evidence: `base.css:17-25`; `AgentTranscriptPanel.svelte:772,784,821,830,835`; `sidebar.css:230-231,248`; `SplitPane.svelte:694,920,961,979,1025,1075`; `history-dashboard.css:15-112,156,181`

### [P3] Icon buttons without `font: inherit` render in the UA default font (Arial / GTK font) instead of DM Sans
- Area: `app/frontend/src/styles/ai-hub.css:102-110` (`.settings-inline-btn`)
- Platform: all
- Symptom: The font-size stepper buttons (− / +), `.sidebar-toggle` (◀/▶), `.sidebar-add-btn` (+), `.header-btn` glyphs, `.agent-btn` and the collapsed-rail glyphs use the browser's default button font: Chromium/WebView2 → Arial, WebKitGTK → the GTK UI font. Glyph shapes and vertical centring differ from the surrounding DM Sans text; the `+` of "+ New" (styled) and the `+` next to SSH HOSTS (unstyled) do not match.
- Repro: 1. Open the app. 2. Computed `fontFamily` of `.settings-inline-btn`, `.sidebar-toggle`, `.sidebar-add-btn`, `.header-btn` = `Arial`; of `.add-btn`/`.modal-primary-button` = DM Sans.
- Cause: `base.css` has no element reset for `button`; only `.add-btn`, `.modal-primary-button`, `.ctx-item`, `.restore-badge` and `AgentWorkspaceView` set `font-family`/`font: inherit`.
- Fix sketch: Add `button, input, select, textarea { font: inherit; }` to the reset in `style.css:1-5`; keep `--font-mono` on the type badge.
- Evidence: `style.css:1-5`; `ai-hub.css:102-110`; `sidebar.css:41-55`; `style.css:155-168` (`.header-btn`); `AgentTranscriptPanel.svelte:775`

### [P3] No app-wide `:focus-visible` style; sidebar navigation relies on the UA ring on tabindex DIVs, inputs replace the ring with a low-contrast border
- Area: `app/frontend/src/styles/sidebar.css:148-163` (`.sidebar-heading`)
- Platform: all (ring colour is platform chrome: Chromium dual ring, WebKitGTK theme colour)
- Symptom: Tabbing through the sidebar focuses `DIV.sidebar-heading` elements with the browser's `outline: auto 1px` only; pane buttons (`.header-btn`) get the UA ring at 3 px; 19 rules set `outline: none` on inputs and swap it for `border-color: #63b3ed55` (≈1.3:1 against the surface) so the focused search/filter field is hard to spot; only `AgentWorkspaceView.svelte:303` defines a visible 2 px accent ring. Four different focus looks in one app.
- Repro: 1. Click the brand, press Tab ×6: focus order `sidebar-heading`, `sidebar-heading`, `sidebar-add-btn`, … all with `outline: auto 1px` and `:focus-visible` true. 2. `.settings-inline-btn` focused: `outline: none`. 3. Code: `history-dashboard.css:101,120`, `template-manager.css:65`, `sidebar.css:424` replace the ring with `#63b3ed55`/`--border-accent`.
- Cause: No global `:focus-visible` rule; `outline: none` in `style.css`, `sidebar.css`, `main.css`, `overlays.css`, `history-dashboard.css`, `template-manager.css`, `workflow-builder.css`, `recording-player.css`, `SplitPane.svelte`, `MarkdownNotes.svelte`, `WorkflowPicker.svelte`, `TranscriptViewerModal.svelte`, `SecretUnlockGate.svelte`.
- Fix sketch: One global `:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }` in `style.css`, delete the per-component `outline: none` (or scope it to `:focus:not(:focus-visible)`); promote the heading DIVs to `<button>`s (also covered by keyboard-a11y).
- Evidence: `sidebar.css:148-163`; `AgentWorkspaceView.svelte:303`; `grep -rn "outline: none" src` (19 hits); live focus sequence

### [P3] Hover feedback on most button classes is below a perceivable step; disabled opacity differs per component
- Area: `app/frontend/src/styles/ai-hub.css:111`
- Platform: all
- Symptom: `.settings-inline-btn:hover` and `.agent-btn:hover` lighten the background by `rgba(255,255,255,0.06)` (1.16:1 against the card), sidebar rows/headings by `--accent-glow` (1.20:1 on void), the collapsed rail only scales the glyph; on a 125 % VDI monitor these states are invisible, so the only hover cue is the cursor. Disabled buttons use five different opacities (`.settings-inline-btn` .6, `.sidebar-answer-btn` .5, `AgentWorkspaceView` .45, `.agent-link` .4, `.pagination` .3); `.header-btn`/`.sidebar-toggle` have no disabled style; `.sidebar-toggle` is the only class with `transform: scale` hover/active.
- Repro: computed from the rules (confidence high); live: `.settings-inline-btn` hover bg `rgba(255,255,255,0.06)`, `.header-btn` hover `rgb(35,40,64)` on `rgb(17,20,32)` (≈1.4:1, acceptable).
- Cause: Per-component hover rules (`ai-hub.css:111`, `AgentTranscriptPanel.svelte:776`, `sidebar.css:164-167,284-287`) with no shared hover token; disabled rules in `ai-hub.css:112`, `sidebar.css:250`, `AgentWorkspaceView.svelte:302`, `AgentTranscriptPanel.svelte:801`, `history-dashboard.css:223`.
- Fix sketch: Define `--state-hover: rgba(99,179,237,0.14)` plus a border change to `--border-accent` for hover and one disabled opacity (0.45), applied from a shared `.btn` base class.
- Evidence: `ai-hub.css:102-112`; `AgentTranscriptPanel.svelte:775-778,801`; `sidebar.css:59-68,164-167,247-250,284-287`; `history-dashboard.css:223-224`

### [P3] Sub-10 px text and 8 %-alpha borders: badge and labels too small at 100 %, card borders invisible at 1×
- Area: `app/frontend/src/style.css:124` (`.terminal-type-badge` 0.58rem)
- Platform: all
- Symptom: The pane type badge is 9.28 px JetBrains Mono uppercase; the agent panel uses 10 px (`.agent-panel-status`, `.agent-group-label`, `.agent-row-dim`) and 9 px (`.agent-op`), SplitPane badges 10 px (six rules), history dashboard 10 px (seven rules). `--border-subtle` (`rgba(99,179,237,0.08)`, 1.11:1) on cards and `.agent-btn`/`.settings-inline-btn` is indistinguishable from the background at 1×: the Settings cards read as borderless and the stepper buttons as plain text; at 2× the hairline becomes faintly visible, so the two DPIs look like different designs.
- Repro: 1. Live @1x: `.terminal-type-badge` font-size 9.28 px; `.ai-hub-card`/`.settings-inline-btn` border `1px rgba(99,179,237,0.08)`. 2. Compare `theme-dpi-settings-emoji-dsf2.png` (border just visible) with `theme-dpi-terminals-1280-dsf1.png` (sidebar separators barely visible).
- Cause: `style.css:124`; `AgentTranscriptPanel.svelte:756,793,808,817,819`; `SplitPane.svelte:744-1081` (`font-size: 10px` ×6); `history-dashboard.css:177-411`; `base.css:15` `--border-subtle` at 0.08.
- Fix sketch: Floor UI text at 11 px (0.69rem) and badges at 10.5 px; raise `--border-subtle` to ≈0.14 and `--border-dim` to ≈0.24 so 1 px hairlines survive 1× rendering.
- Evidence: `style.css:120-134`; `base.css:15-16`; contrast table above

### [P3] 1 px / 2 px hairlines and the 6 px scrollbar land on fractional device pixels at 125 % / 150 %
- Area: `app/frontend/src/style.css:144-150` (`.header-sep` 1 px)
- Platform: windows-webview2 (125/150 % are the common VDI scales), linux-webkitgtk at fractional scale
- Symptom: `.header-sep` (1 px), the active-nav bar (`2px` + glow), the `.terminal-wrapper.active-terminal` inset shadow (`0 2px 0 0`), `.drag-over-top/bottom` (2 px) and the 6 px global scrollbar thumb map to 1.25 / 2.5 / 7.5 device pixels; WebKit/Chromium snap them per element, so neighbouring panes show different separator thickness and the active-pane bar alternates between crisp and blurred depending on its y offset.
- Repro: code reasoning, confidence low–medium (no fractional-DPR screenshot of the chrome; the 1.25 run showed xterm itself rounding to device pixels: row height 23.188 = 29/1.25, the chrome does not).
- Cause: Fixed px sizes in `style.css:94-110,144-150,232-238`, `sidebar.css:98-107,175-185`, `style.css:23-39`.
- Fix sketch: Draw hairlines with `box-shadow: 0 0 0 1px` or `border` on integer-aligned elements, make the scrollbar 8 px, and keep indicator bars at whole-number multiples that look the same at 1×/1.25×/1.5× (e.g. 2 px → `max(2px, 0.125rem)` and `transform: translateZ(0)` on the pane header).
- Evidence: `style.css:23-39,86-110,144-150,232-238`; `sidebar.css:98-107,175-185`

### [P3] No `color-scheme: dark`: native popups and UA controls render light inside the dark UI
- Area: `app/frontend/src/style.css:8-16`
- Platform: windows-webview2, macos-webkit (Chromium/WebKit draw the `<select>` popup, spinners and non-styled scrollbars from `color-scheme`); linux-webkitgtk follows the GTK theme
- Symptom: The closed `<select>` is styled (`appearance: none`, custom arrow), but the dropdown list of the terminal-type selector, Settings (tmux mode, renderer, language) and the SSH profile editor is the platform's light list (white background, black text) on the dark app; the same for `<input type=number>` spinners. `prefers-color-scheme` is neither read nor declared and the computed `color-scheme` is `normal`.
- Repro: 1. Live: `getComputedStyle(document.documentElement).colorScheme === 'normal'`; `emulateMedia({colorScheme:'light'})` changes nothing (verified: no light rules leak). 2. Native popup appearance: code reasoning, confidence medium (platform chrome).
- Cause: No `color-scheme: dark` on `:root`/`html` and no `<meta name="color-scheme">` in `index.html`; no `@media (prefers-color-scheme)` rules (intended, dark-only).
- Fix sketch: Add `color-scheme: dark;` to `:root` in `base.css` (or `<meta name="color-scheme" content="dark">`) so WebView2/WebKit pick dark UA widgets; keep the app dark-only.
- Evidence: `base.css:10-37`; `index.html:1-10`; `main.css:48-70` (select styling of the closed state only)

### [P3] Global scrollbar: 6 px thumb at 20 % alpha on a transparent track is hard to find and grab
- Area: `app/frontend/src/style.css:23-39`
- Platform: all (WebKitGTK and WebView2 honour `::-webkit-scrollbar`)
- Symptom: Long Settings/History/Agent-panel lists scroll, but the only scrollbar is a 6 px thumb in `rgba(99,179,237,0.2)` (≈1.3:1 against the surface) with no track; at 125 %+ it is 7.5 device px and easy to miss with a mouse. The xterm viewport's native bar is correctly hidden in favour of xterm 6's slider (themed, 8 px, hidden when there is no scrollback).
- Repro: code reasoning, confidence medium; live: `.ai-hub` `offsetWidth − clientWidth = 0` (overlay-style thin bar), xterm `.slider` 0×0 with empty scrollback, `.xterm-viewport` `scrollbar-width: none` applied.
- Cause: `style.css:23-39` (`width: 6px`, thumb 0.2 alpha, track transparent).
- Fix sketch: 8–10 px, thumb `rgba(99,179,237,0.35)`, hover 0.6, track in `--bg-deep`; add `scrollbar-color`/`scrollbar-width: thin` for the non-WebKit path.
- Evidence: `style.css:23-39,55-62`; `terminalActions.js:28-32,121`

## Not found / verified OK

- `prefers-color-scheme: light` emulation changes no computed style (no light-theme rules leak; the app is consistently dark-only).
- `.xterm-viewport` background equals `--bg-void` and the native viewport scrollbar is hidden (`scrollbar-width: none` + `::-webkit-scrollbar 0`); no black strip below the last row.
- xterm 6 slider colours come from `XTERM_THEME` (`rgba(99,179,237,…)`) and the slider is 0×0 when there is no scrollback; it is not the source of the white line.
- xterm 6 itself listens to `(resolution: Xdppx)` and re-measures on a DPR change (confirmed: row height 20 → 22.5 → 23.19 at DPR 1/2/1.25); only Mimir's refit is missing (finding 3).
- With network, both Google-hosted families load (`document.fonts.check` true for JetBrains Mono and DM Sans); no CSP violations in the console.
- Nav-rail glyphs (▸ → ✱ ⌂ ⚙ ⚛ ≡) are monochrome, all 14.4 px, 8–15 px wide, centred in the 45 px rail at 2×; the sidebar is consistent with commit `385f65d`.
- The agreed status palette (`#7ee787`, `#e3b341`, `#d2a8ff`, `#63b3ed`) is used consistently wherever it appears (22/13/5/21 occurrences); only red drifts (finding 9).
- Checkboxes use `accent-color: var(--accent)` in Settings and modals; the `<select>` closed state is themed with an SVG arrow in the token colour.
- `::selection` and xterm `selectionBackground` use the same accent alpha family.
- Nothing in `main.go` forces a light webview theme (`Windows.Theme` / `WebviewIsTransparent` not set); only the background colour is off (finding 4).
