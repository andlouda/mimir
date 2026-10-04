# 99 — Summary (ranked across domains)

Audit of 2026-10-04 on `main` (v0.8.11 plus the unmerged PRs #45–#48, which
are referenced where they change a finding). Nine domains, 149 findings:
22 P1, 71 P2, 56 P3. Roughly two thirds were reproduced live against the
mocked production build; the rest are code reasoning with stated
confidence.

| Domain | P1 | P2 | P3 | File |
|---|---|---|---|---|
| forms-modals | 4 | 10 | 6 | `findings-forms-modals.md` |
| state-sync | 5 | 6 | 4 | `findings-state-sync.md` |
| platform | 3 | 7 | 6 | `findings-platform.md` |
| layout-panes | 2 | 8 | 6 | `findings-layout-panes.md` |
| keyboard-a11y | 2 | 9 | 8 | `findings-keyboard-a11y.md` |
| terminal-render | 1 | 8 | 5 | `findings-terminal-render.md` |
| theme-dpi | 0 | 7 | 9 | `findings-theme-dpi.md` |
| i18n-copy | 0 | 3 | 10 | `findings-i18n-copy.md` |
| side-panels | 5 | 13 | 2 | `findings-side-panels.md` |

## Fix first (P1, ordered by impact × ease)

1. **Panes shrink exponentially** — "+ New", session restore and
   minimize→restore each wrap the whole layout in a new root split at 50 %
   (`terminalActions.js:441`, `App.svelte:377`, `terminalActions.js:617`).
   Six terminals: 29/29/63/129/262/528 px; ten: three panes at 0 px. This is
   the "I can only have two open" complaint, not the font size.
   *Fix:* insert into the current leaf's parent (balanced split) and give new
   siblings an equal share; clamp a pixel minimum. — layout-panes
2. **Split layout is never persisted** — only per-terminal fields are saved
   (`sessionActions.js:24–36`); every restart rebuilds a horizontal chain.
   *Fix:* persist `layoutTree` (ids → saved terminal indices) and restore it.
   — layout-panes
3. **Escape goes to the shell instead of closing overlays** — xterm stops
   propagation of every key it handles, so the window-level Escape handler
   never runs while a terminal has focus; Escape with the search bar or
   context menu open sends `\x1b` to Claude Code / vim and leaves the overlay
   up. *Fix:* handle Escape (and the overlay stack) inside the xterm custom
   key handler. — keyboard-a11y
4. **Ctrl+Shift+F leaves focus in the terminal** — the query is typed into
   the shell; Enter would run it (`terminalSearchActions.js:4`,
   `SplitPane.svelte:523`). *Fix:* focus the search input on open, restore
   xterm focus on close. — terminal-render, keyboard-a11y (same defect)
5. **Dropping a file onto the window navigates the webview away** — nothing
   cancels `dragover`/`drop` globally; WebView2 and WebKitGTK load `file://`
   and unmount the app, every terminal is lost. *Fix:* window-level
   `dragover`/`drop` preventDefault (keep the folder-row drop targets).
   — platform
6. **Master password can be typed into the shell behind the gate** — the
   secret gate does not trap focus; keystrokes reach the active xterm.
   *Fix:* focus trap + `inert` on the app while the gate is shown.
   — forms-modals
7. **"Insert into Terminal" from the AI panel executes the command
   immediately** (newline appended). *Fix:* insert without newline, let the
   user press Enter. — forms-modals
8. **Ctrl+Shift+T on a non-terminal page creates a dead, persisted pane**
   (`terminalActions.js:227` early return before wiring). *Fix:* switch to
   the terminals page first or defer creation. — state-sync
9. **Closing a pane during its own initialisation re-persists it** and
   leaves `activeTerminalId` dangling, agent watch leaks
   (`terminalActions.js:450–452`). — state-sync
10. **Agent-panel note lands on the next pane's session** after a quick
    switch (debounce keeps the old text, writes with the new key,
    `AgentTranscriptPanel.svelte:75`). *Fix:* flush/cancel the debounce on
    terminal change, key the timer by session. — state-sync
11. **Recording flag lost while the pane prints** → recording cannot be
    stopped. — state-sync
12. **Webview reload restores the saved session a second time** on top of
    the live terminals (`GetLoadedSessionData` is a static snapshot).
    *Fix:* clear the snapshot after the first restore / guard by a flag.
    — state-sync
13. **Approval dialog can be dismissed and never reopened**; the workflow
    stays "Waiting for approval". — forms-modals
14. **Template prompt: Enter twice runs the template twice** (no
    double-submit guard). — forms-modals
15. **Linux release linked against webkit2gtk-4.0** (`release.yml:35,74`,
    no `-tags webkit2_41`): cannot start on Fedora 40+, Debian 13, Ubuntu
    25.04+. *Fix:* build with `webkit2_41` (and document the 4.1 dependency).
    — platform
16. **`GTK_IM_MODULE=gtk-im-context-simple` forced** when unset: no
    CJK/complex-script input on GNOME/KDE where ibus/fcitx work without the
    variable; the umlaut fix only helps users who had nothing exported.
    *Fix:* make it a setting (default: only when no IM module is active).
    — platform
17. **Notes: "Back" on an unsaved note resurrects a nameless ghost editor**
    and then calls `SaveNote(null, …)`; closing the panel while dirty drops
    the edit silently (`MarkdownNotes.svelte:188/74/245`). — side-panels
18. **Activity log blanks to "0 entries"** when two entries share kind,
    second-precision timestamp and title (Svelte `each_key_duplicate` in the
    production build, `ActivityLogViewer.svelte:228`; backend stamps RFC3339
    seconds at `app.go:509`). *Fix:* key by index or a composite id.
    — side-panels
19. **GIF export: no progress, double-click starts two exports**, result or
    error only appears later as the red banner on the Terminals page
    (`RecordingPlayer.svelte:280`). — side-panels
20. **FileBrowser shows a stale listing under the current path** when
    responses arrive out of order (`FileBrowser.svelte:75`), and has no
    loading state. — side-panels
21. **Agent panel never auto-refreshes while working**: every agent-state
    event restarts the 6 s interval (`AgentTranscriptPanel.svelte:169`;
    0 refreshes in 14 s live). — side-panels
22. **"To notes" twice within a second overwrites the first note** (same
    second-precision file name, `AgentTranscriptPanel.svelte:345`).
    — side-panels

## Themes behind the P2s

- **Focus and overlays** (keyboard-a11y, forms-modals): most modals neither
  take focus nor close on Escape; no focus trap/restore except the
  transcript viewer; glyph-only buttons without accessible names; held keys
  repeat (Ctrl+Shift+T opened three terminals); CapsLock / non-Latin layouts
  break letter-keyed shortcuts.
- **Fonts and metrics** (theme-dpi, terminal-render, platform): fonts were
  fetched from Google at every launch and cells measured before the swap;
  DPR changes without a resize never refit the terminal (bottom rows hidden,
  PTY size stale). PR #48 bundles the fonts; the CSP `font-src` and the
  Google `<link>`s were fixed in that PR after this finding (`9ab291b`).
  The 1 px white line at the right edge of every pane is xterm 6's overview
  ruler border, now themed in PR #48.
- **Search** (terminal-render): `findNext` without decorations — single
  match, no highlight-all, no counter, no "no match" state.
- **Side panels vs window width** (layout-panes): notes and agent panel are
  not clamped; a stored 800 px notes width leaves a 0 px terminal area at
  1024 px; the restore summary (z-index 60) sits above modals (30) and the
  search bar (20).
- **Shell integration overwrites the user's prompt** (platform): PowerShell
  `function prompt…; cls` replaces the backend's cwd beacon prompt, so cwd
  and history stop working on Windows; bash/zsh PS1/PROMPT are overwritten
  after the user's rc (Starship/oh-my-posh users lose their prompt).
- **Colour and contrast** (theme-dpi): `--text-muted` 2.9:1 and
  `--text-secondary` 3.9–4.3:1 fail AA on raised surfaces; three different
  reds; colour emoji in Settings/FileBrowser/DotEnv cards against the
  monochrome sidebar; no global `:focus-visible` (19× `outline: none`);
  window `BackgroundColour #1b2636` vs `--bg-void #0c0e14` flashes on
  launch/resize; no `color-scheme: dark` (light native popups).
- **Errors that vanish** (state-sync, forms-modals, side-panels): the error
  banner only renders on the Terminals page; history errors render as "No
  commands found"; the consent "Not now" is not persisted; SSH restore errors are swallowed; the AI
  modal closed while a request is pending throws a TypeError banner.
- **i18n** (i18n-copy): a raw key in the restore badge, hard-coded English
  in FileBrowser errors and reconnect labels, German overflow in badges and
  settings cards; "Freigabe" vs "Erlauben" naming consistent, pluralisation
  mostly fine.
- **Wayland / GPU** (platform, on PR #48): `WEBKIT_DISABLE_DMABUF_RENDERER`
  is honoured by WebKitGTK < 2.46 only (the variable was removed with the
  2.46 renderer rewrite) — harmless but ineffective on current distros;
  WebGL needs the GPU policy ≠ never. The PR text should say so.

## What was verified OK (selection)

No listener or ResizeObserver leaks on pane close; page switch keeps the
xterm node, buffer and scroll position; minimized panes keep their output;
resize IPC is debounced to one call per burst; Ctrl+Shift+C / OSC 52 /
context-menu copy and paste work without double paste; CJK width is
correct; store churn per output chunk is cheap (~5 µs); GTK's reserved
Ctrl+Shift+U is covered by the O alias; digits use `event.code`;
TemplatePicker, WorkflowPicker and TranscriptViewerModal handle focus and
Escape correctly; the status palette is used consistently in badges,
sidebar and workspace.

## Suggested order of fix PRs

1. Layout: balanced splits + persisted layout + side-panel clamp (P1 1, 2; P2).
2. Keyboard: Escape/overlay stack inside the xterm key handler, search
   focus, key repeat guard, modal focus traps and Escape (P1 3, 4, 6, 13, 14).
3. State: dead pane on Ctrl+Shift+T, close-during-init, note debounce,
   recording flag, double restore, error banner on every page (P1 8–12).
3b. Panels: notes ghost editor + dirty close, activity-log keys, GIF export
   progress and double-submit, FileBrowser request ordering + loading state,
   agent-panel refresh timer, note file names with milliseconds (P1 17–22).
4. Platform: global drop guard, webkit2_41 release build, IM-module setting,
   PowerShell/bash prompt integration that keeps the user's prompt (P1 5, 15, 16).
5. Polish: contrast tokens, focus-visible, monochrome card icons,
   BackgroundColour, color-scheme, search decorations, i18n leftovers.
