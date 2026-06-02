# Terminal Stability Notes

## Purpose

This document captures the terminal lifecycle bugs that caused repeated UI instability,
especially with multiple visible terminals, SSH tabs, and tmux-backed sessions.

The goal is to keep these issues from reappearing during future refactors.

## Main Failure Modes We Hit

### 1. Re-opening an existing xterm instance during layout changes

Root cause:

- The frontend re-rendered terminal containers when the split tree changed.
- Existing xterm instances were effectively opened again against new DOM nodes.
- This became unstable as soon as multiple visible terminals existed.

Observed symptoms:

- UI instability after opening the second or third terminal
- crashes or partial UI reloads
- terminal listeners disappearing after layout updates

Fix:

- Existing terminal DOM is now re-attached instead of calling `terminal.open(...)` again.
- See [`frontend/src/App.svelte`](frontend/src/App.svelte:1512).

Rule:

- Never call `open()` again for an already attached xterm instance just because the layout changed.
- Move the existing `term.terminal.element` into the new container instead.

### 2. Fresh SSH tabs sharing the same tmux session

Root cause:

- Fresh SSH tabs and restore/reconnect flows were too similar.
- tmux attach behavior reused an existing session unexpectedly.

Observed symptoms:

- second SSH tab on the same host showed the other tab's shell state/history
- closing one SSH tab could effectively impact the other

Fix:

- Fresh SSH terminals now create a new tmux session.
- Restore/reconnect paths re-attach to an existing tmux session explicitly.
- Tabs track whether they "own" the tmux session.
- Only owned tmux sessions are killed on explicit close.
- See [`frontend/src/App.svelte`](frontend/src/App.svelte:646).

Rule:

- New terminal creation and restore/reconnect are different lifecycle paths.
- Do not collapse them into one generic `tmux -A` path.

### 3. Frontend reloads amplifying broken terminal state

Root cause:

- The frontend called `SaveCurrentSession()` in `onDestroy`.
- During dev reloads or UI crashes, backend terminals were still alive.
- The destroyed frontend saved partial or invalid terminal state.
- On the next load, old backend sessions and restored frontend sessions could drift apart.

Observed symptoms:

- many `No listeners for event 'terminal-output-*'`
- duplicated or stale terminal state after reload
- "zombie" terminal behavior

Fix:

- Removed frontend-side session save from `onDestroy`.
- Real app-close persistence remains handled in the backend lifecycle.
- See [`frontend/src/App.svelte`](frontend/src/App.svelte:1728).

Rule:

- Do not persist terminal session state from a generic frontend teardown hook.
- Session persistence belongs to an actual application-close path, not a component-destroy path.

### 4. Single terminal failures crashing the whole UI path

Root cause:

- terminal `write`, `fit`, `resize`, `open`, and `dispose` were not consistently isolated
- one broken pane could cascade into broader UI failure

Fix:

- Added guarded helpers for:
  - attach
  - write
  - fit/resize
  - dispose
- See [`frontend/src/App.svelte`](frontend/src/App.svelte:1476).

Rule:

- Terminal lifecycle operations must fail per-terminal, not globally.
- A single bad pane must degrade locally and log, not destabilize the entire app.

## Current Frontend Lifecycle Rules

### Terminal creation

- Create the backend session first.
- Insert the new leaf into the layout tree before DOM-based attach logic.
- Create the xterm instance once.
- Register output/closed/disconnected event listeners before marking the terminal as ready.
- Call:
  1. `ConfirmFrontendReady(id)`
  2. `InitializeTerminal(id)`

### Layout changes

- Layout tree changes may recreate container DOM.
- Existing xterm instances must be re-attached by moving the existing element.
- Do not re-open xterm instances during split/minimize/remove/reflow unless the terminal is truly new.

### SSH lifecycle

- Fresh SSH tab:
  - create fresh tmux session
  - mark `tmuxOwned = true`
- Restored SSH tab:
  - re-attach to stored tmux session
  - mark `tmuxOwned = false`
- Reconnected SSH tab:
  - re-attach to existing tmux session
  - do not treat it like a brand-new shell

### Close behavior

- Explicit close:
  - close backend terminal
  - only kill tmux if this tab owns it
- Disconnected SSH close:
  - fully remove SSH terminal from backend manager
- Fallback cleanup exists, but should remain a fallback only

## Known Logging Signals

### `No listeners for event 'terminal-output-*'`

Interpretation:

- Usually means backend output is still being emitted while the frontend is no longer correctly subscribed.
- This is often a symptom of a frontend reload/crash or lifecycle drift.

It does **not** automatically mean the backend PTY logic is wrong.

If this appears again:

1. Check whether the frontend reloaded.
2. Check whether an xterm attach/open error happened first.
3. Check whether terminals were restored from stale frontend state.

## Regression Checklist

Run this checklist before merging terminal-layout, SSH, or session-persistence changes.

### Local terminals

1. Open 1 local terminal.
2. Open a second local terminal.
3. Open a third local terminal.
4. Split one horizontally.
5. Split one vertically.
6. Close the first visible terminal.
7. Close the active terminal.
8. Minimize and restore a terminal.
9. Rename a terminal.

Expected:

- no UI crash
- no full frontend reload
- active terminal state remains valid

### SSH terminals

1. Open SSH tab to host A.
2. Open second SSH tab to host A.
3. Confirm shell state is independent.
4. Close only one tab.
5. Reconnect a disconnected SSH tab.

Expected:

- no shared shell session on fresh tabs
- closing one tab does not kill the other
- reconnect re-attaches cleanly

### Mixed terminals

1. Open 2 local terminals.
2. Open 2 SSH terminals.
3. Reorder panes via drag/drop.
4. Resize split dividers.
5. Close terminals in random order.

Expected:

- no listener drift
- no layout corruption
- no xterm re-open instability

### Reload / restart sanity

1. Start `wails dev`.
2. Open several terminals.
3. Restart the frontend/app intentionally.
4. Confirm stale frontend teardown does not corrupt saved session state.

## Refactoring Guidance

This area is still too concentrated in [`frontend/src/App.svelte`](frontend/src/App.svelte:1).

Priority extraction targets:

- terminal lifecycle helpers
- SSH/tmux lifecycle handling
- layout tree management
- event registration and cleanup

Current split:

- [`frontend/src/lib/terminals/layoutTree.js`](frontend/src/lib/terminals/layoutTree.js:1)
- [`frontend/src/lib/terminals/tmuxLifecycle.js`](frontend/src/lib/terminals/tmuxLifecycle.js:1)
- [`frontend/src/lib/terminals/xtermLifecycle.js`](frontend/src/lib/terminals/xtermLifecycle.js:1)

Still recommended next extraction:

- `terminalEvents`
- `sshTerminalLifecycle`

## Bottom Line

The most important invariant is:

> A terminal instance is created once, attached carefully, moved safely, and closed explicitly.

Do not let layout changes, frontend teardown, or SSH restore behavior blur those boundaries again.
