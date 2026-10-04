# Findings — state-sync (Wails events → UI)

Scope: `terminal-output/closed/disconnected/prompt-<id>`, `agent-state-<id>`,
`update-progress`, session restore/persist, agent panel follow, optimistic UI.
Live repros ran against the mocked backend (`tests/e2e/fixtures/mimirApp.js`,
`window.__emitMimir`) with a throw-away spec that has been deleted; items the
mocks cannot drive (real PTY lifetime, SSH, webview reload) are code reasoning.
Note: this branch does have per-terminal restore error handling
(`restoreWithRetry`, `notRestored`), contrary to the audit brief — see the SSH
restore finding for the gap that remains.

### [P1] Ctrl+Shift+T on any non-terminal page creates a dead, persisted pane
- Area: `app/frontend/src/lib/actions/terminalActions.js:227`
- Platform: all
- Symptom: On Settings (or any other page) Ctrl+Shift+T shows the banner "Failed to find terminal element for ID: 2"; back on Terminals a second pane exists that accepts keystrokes but never prints anything, and it is restored again on the next start.
- Repro: 1. Open Settings. 2. Press Ctrl+Shift+T. 3. Return to Terminals. Mocked run: `ConfirmFrontendReady`/`InitializeTerminal` were never called for id 2, `UpdateTerminalState(2)` was, 2 panes rendered. (confidence high)
- Cause: `createTerminalInstance` returns early when `document.getElementById('terminal-<id>')` is missing and the terminal is not minimized (`:227-231`) — before `EventsOn('terminal-output-…')` (`:233`), `ConfirmFrontendReady`/`InitializeTerminal` (`:266-267`) and `startAgentWatch`. The backend never starts the read goroutine (`app/terminal/terminal.go:416` waits on the ready channel) while `terminal.onData` (`:201`) still forwards input. `addTerminal` then sets it active and persists it (`:451-452`). `keyboardShortcuts.js:166` fires regardless of `currentPage`.
- Fix sketch: In `addTerminal` switch to the terminals page (or ignore the shortcut) before creating; or treat "no element" like the minimized path (subscribe events, confirm ready, attach on first `reinitializeTerminals`).
- Evidence: `terminalActions.js:215-231, 233-270, 450-452`; `keyboardShortcuts.js:166-168`; `terminal/terminal.go:402-416`.

### [P1] Closing a pane during its own initialisation re-persists it and leaves `activeTerminalId` dangling
- Area: `app/frontend/src/lib/actions/terminalActions.js:450`
- Platform: all
- Symptom: After closing a pane that was still starting (slow tmux/SSH bootstrap), no pane is highlighted as active, "Please select a terminal first." appears on AI/template actions, and the closed terminal is restored again on the next start.
- Repro: 1. Make `InitializeTerminal` take ~700 ms. 2. "+ New". 3. Click the new pane's ✕ immediately. Mocked run: `RemoveTerminalState(2)` at t, `UpdateTerminalState(2)` at t+130 ms, `.active-terminal` count 0 with 1 pane left. (confidence high)
- Cause: `createTerminalInstance` keeps running after `finalizeTerminalRemoval` removed the terminal; `addTerminal` then does `activeTerminalId.set(id)` and `persistTerminalState(newTerminal)` (`:451-452`), re-adding the id to `activeTerminalStates` (`app/app.go:260`). `startAgentWatch(id)` (`:269`) also leaks: its stop handler is pushed onto `newTerminal.cleanupHandlers`, but `cleanupTerminalResources` replaced the store copy's array with a fresh one (`:86`), so nothing iterates it again.
- Fix sketch: After each `await` in `createTerminalInstance`/`addTerminal`/`splitTerminal` check that the id is still in `terminals` (or keep a `closing` set) and bail out; have `finalizeTerminalRemoval` also stop the agent watch by id.
- Evidence: `terminalActions.js:76-110, 266-270, 450-454, 554-575`; `app.go:260-282, 493-497`.

### [P1] A note typed in the agent panel is saved on the next pane's session after a quick switch
- Area: `app/frontend/src/lib/AgentTranscriptPanel.svelte:75`
- Platform: all
- Symptom: Type a note for agent A, click pane B within 600 ms (the unpinned panel follows): the note is written to B's session, A's note is lost, and the note box stays open under B's header still showing A's text.
- Repro: 1. Two panes with agents, `agent-state-1/2` carrying different `sessionFile`s. 2. Open the panel from pane 1, click "Note", type. 3. Click pane 2. Mocked run: `SetAgentSessionAnnotation('local|/s/two.jsonl', '', 'note for ONE', false)`; textarea still open with "note for ONE" under "BASH 2". (confidence high)
- Cause: `noteChanged` schedules a 600 ms timer whose closure reads the live `terminalId` and `noteDraft`; `resetFor()` (`:185-188`) resets neither `noteOpen`, `noteDraft` nor `noteTimer`; `$: if (!noteOpen) noteDraft = …` (`:65`) never runs while the box is open. The follow logic is `agentActions.js:363-370`.
- Fix sketch: Capture `terminalId` and `sessionKey(agent)` when scheduling and pass them to `setSessionAnnotation`; flush or cancel the timer and close the note box in `resetFor()`.
- Evidence: `AgentTranscriptPanel.svelte:62-79, 150, 185-188, 447-449`; `agentActions.js:140-152, 363-370`.

### [P1] Recording toggled while the pane prints output: the flag is lost and the recording can no longer be stopped
- Area: `app/frontend/src/lib/actions/terminalActions.js:540`
- Platform: all
- Symptom: Click ⏺ on a busy pane: the button never turns red. Clicking again calls `StartRecording` a second time (the real backend answers "already being recorded", which lands in the console only); there is no UI path to `StopRecording`, so the recording keeps growing until the terminal closes.
- Repro: 1. Make `StartRecording` resolve after 300 ms. 2. Click ⏺ on pane 1. 3. Emit `terminal-output-1` during the wait. Mocked run: `.record-btn.recording` count 0, after a second click `start: 2, stop: 0`. (confidence high; backend error path by code reasoning)
- Cause: `term` is captured before `await StartRecording` (`:536, :545`); every output chunk replaces the object (`:236-240`), so `term.recording = true` (`:546`) mutates a stale copy and `terminals.update(list => [...list])` republishes the unchanged one. Errors are swallowed (`:549-551`). Backend: `terminal/terminal.go:596-598`.
- Fix sketch: Update the flag through `terminals.update(map …)` after the await; surface the error in `errorMessage`; optionally derive the flag from the backend.
- Evidence: `terminalActions.js:233-242, 535-552`; `terminal/terminal.go:594-599`.

### [P1] A webview reload restores the saved session a second time on top of the live terminals
- Area: `app/frontend/src/App.svelte:453`
- Platform: all (reachable through the default context-menu "Reload", see layout/keyboard findings)
- Symptom: After a reload the saved layout comes back, every saved shell is started again (two shells / two tmux attach clients per entry), terminals opened since start are gone from the UI but keep running, and the old ids stay in the session file.
- Repro: code reasoning, confidence high
- Cause: `GetLoadedSessionData` returns the start-up snapshot forever (`app/app.go:94, 239-242`); `onMount` restores it unconditionally. The previous frontend's terminals are never closed (no `RemoveTerminalState`, no `CloseTerminal`), so `activeTerminalStates` holds old and new ids and `SaveCurrentSession` (`:244-258`) writes both; `dedupeSavedSessionTerminals` collapses them on the next start only because the resumeId is reused (`util.js:138-152`).
- Fix sketch: Hand the snapshot out once (clear it after the first `GetLoadedSessionData`) and, on a repeated call, return the live terminals so the frontend re-attaches instead of re-spawning; or disable the Reload entry.
- Evidence: `App.svelte:427-481`; `app.go:94, 239-258, 260-282`.

### [P2] Agent panel shows the previous pane's transcript under the new pane's header and never loads the new one
- Area: `app/frontend/src/lib/AgentTranscriptPanel.svelte:256`
- Platform: all
- Symptom: Switch panes while the first transcript is still loading: the header changes to "BASH 2 · /repo" immediately, then terminal 1's messages appear below it; terminal 2's transcript is not fetched until an agent-state event or a manual ↻.
- Repro: 1. `GetAgentTranscriptJSON` slow (1.5 s) for id 1, fast for id 2. 2. Open the panel from pane 1, click pane 2 within a second. Mocked run after 2 s: header "BASH 2", body contains ANSWER-FROM-TERMINAL-1, not -2. (confidence high)
- Cause: `refresh()` returns when `loading` is already true (`:256`) and has no stale-response guard, so the late response for id 1 is written into `transcript` after `resetFor()`; the periodic `refreshTimer` only exists while the agent is `working` (`:245`).
- Fix sketch: Capture `const id = terminalId` per request, ignore responses whose id no longer matches, and let a terminal switch bypass the `loading` gate (request counter).
- Evidence: `AgentTranscriptPanel.svelte:150, 185-188, 240-267`.

### [P2] Closing the AI modal while a request is pending re-mounts it with an empty state and raises a TypeError
- Area: `app/frontend/src/lib/modals/AIPanelModal.svelte:58`
- Platform: all
- Symptom: Run "Explain output", close the modal before the answer arrives: when it arrives the error banner shows "Uncaught TypeError: Cannot read properties of undefined (reading 'toUpperCase')" and the answer is lost.
- Repro: 1. `AskAI` resolves after 800 ms. 2. AI → Explain Output → Run. 3. Click ✕ while "Running…". Mocked run: banner as above, no modal. (confidence high)
- Cause: `onClose` sets `$aiPanelState = null`; the late `state = { ...state, loading: false, result }` (`:43/:58`) spreads `null` into `{loading, result}`, `bind:state` (`AppModals.svelte:95`) pushes that back, `{#if aiPanelState}` (`:93`) re-mounts the modal and `state.terminalType.toUpperCase()` (`:98`) throws.
- Fix sketch: Keep a per-run token and drop results after close (or when `state` is null); do not write the result back through the binding once the modal was closed.
- Evidence: `AIPanelModal.svelte:39-62, 98`; `AppModals.svelte:93-96`; `App.svelte:180-193`.

### [P2] Errors raised on non-terminal pages are only shown after returning to Terminals
- Area: `app/frontend/src/lib/views/TerminalsPage.svelte:93`
- Platform: all
- Symptom: An update failure (`update-progress` stage `error`) on Settings, a template-load error on Template Manager, or an SSH connect failure started from the sidebar while on another page shows nothing; the banner pops up later, out of context, when the user opens Terminals.
- Repro: 1. Open Settings. 2. Emit `update-progress` `{stage:'error', error:'boom'}`. Mocked run: `.error-message` count 0 on Settings, "Update failed: boom-test" on Terminals afterwards. (confidence high)
- Cause: The only consumer of `errorMessage` is the banner inside `TerminalsPage.svelte` (`:93-96`), mounted only when `currentPage === "terminals"` (`AppMainContent.svelte:107`); writers live everywhere (`App.svelte:325, 479, 492`, `updateActions.js`, `sshActions.js`).
- Fix sketch: Move the banner to `AppMainContent`/`App` level (or show page-local errors in Settings' update card).
- Evidence: `TerminalsPage.svelte:93-96`; `AppMainContent.svelte:107-113`; `App.svelte:483-495`.

### [P2] SSH restore failures are swallowed: no "not restored" message and the rate-limit retry never sees them
- Area: `app/frontend/src/App.svelte:413`
- Platform: all
- Symptom: After a restart with several saved SSH sessions (or an unreachable host) some SSH panes are simply missing; the `appTerminals.restoreFailed` banner that local terminals get is never shown for them.
- Repro: code reasoning, confidence high
- Cause: `restoreSSHTerminal` catches everything and `console.warn`s (`:413-415`), so `restoreWithRetry` (`:350-360`) never sees the `start_ssh` rate-limit error (`app/app_ssh.go:348`, limiter `ratelimit.go:46`) and `notRestored` (`:455-475`) stays empty for SSH.
- Fix sketch: Re-throw from `restoreSSHTerminal` and let the loop's retry/`notRestored` handling apply to SSH as well.
- Evidence: `App.svelte:348-360, 390-415, 455-475`; `app_ssh.go:348`.

### [P2] Closing a connected SSH pane goes through the "disconnected" path: overlay flash, backend keeps the session
- Area: `app/frontend/src/lib/actions/terminalActions.js:566`
- Platform: all
- Symptom: Clicking ✕ on a live SSH pane shows "Connection lost / Reconnect" for up to 500 ms before the pane disappears; the backend keeps `sessions[id]`, `sshMeta[id]` and `sessionMeta[id]` for the closed terminal.
- Repro: code reasoning, confidence medium-high
- Cause: `removeTerminal` calls `CloseTerminal` for connected SSH (`:566`); the reader goroutine's defer treats any end of an SSH session as a disconnect and emits `terminal-disconnected-<id>` without deleting the maps (`terminal/terminal.go:430-437`); the frontend handler sets `disconnected: true` (`:258-263`) and only the 500 ms fallback (`:567-574`) removes the pane. `CloseSSHTerminalFull` is used only for already-disconnected panes (`:559-565`).
- Fix sketch: For `type === 'ssh'` call `CloseSSHTerminalFull` and finalize directly; or let the backend mark an intentional close so the defer emits `terminal-closed` and clears the maps.
- Evidence: `terminalActions.js:258-264, 554-575`; `terminal/terminal.go:420-446, 639-648, 745-756`.

### [P2] A failed initialisation leaves a dead pane in the layout
- Area: `app/frontend/src/lib/actions/terminalActions.js:480`
- Platform: all
- Symptom: When `ConfirmFrontendReady`/`InitializeTerminal` rejects (backend session gone between `StartTerminal` and init), the banner shows the error but the new pane stays in the layout, silent, not persisted and without `startAgentWatch`; typing into it goes nowhere.
- Repro: code reasoning, confidence medium
- Cause: `addTerminal`'s catch (`:480-484`) only sets `errorMessage`; `createTerminalInstance` already added the terminal (`:186`) and the leaf (`:435-447`) before the awaits at `:266-267`.
- Fix sketch: On error remove the leaf and the store entry (reuse `finalizeTerminalRemoval`) and close the backend terminal.
- Evidence: `terminalActions.js:186, 266-267, 435-447, 480-484`.

### [P3] Attention marker and desktop notification use different "user is looking" predicates
- Area: `app/frontend/src/lib/actions/agentActions.js:209`
- Platform: all
- Symptom: An agent finishes in the active pane while the Mimir window is in the background: a desktop notification is sent, but no green attention badge is set, so after switching back nothing in the UI tells which pane finished.
- Repro: code reasoning, confidence high
- Cause: `handleAgentStateEvent`/`handleTerminalTitle` set `attention` only when `activeTerminalId !== id` (`:209, :256`), while `notifyAgentEvent` also checks `document.hasFocus()` (`:313`).
- Fix sketch: Use the same predicate (pane not active or window not focused) for the marker; clear it on window focus + active pane.
- Evidence: `agentActions.js:207-211, 255-257, 311-313`.

### [P3] Agent detection returning "not detected" closes the open panel
- Area: `app/frontend/src/lib/actions/agentActions.js:70`
- Platform: all
- Symptom: A transient probe miss (process list briefly unavailable, remote `ps` failing, 3-minute liveness tick during a reconnect) removes the badge and closes the panel the user is reading; the state comes back only on the next title/prompt/output trigger.
- Repro: code reasoning, confidence low-medium
- Cause: `runAgentDetection` treats `detected: false` as authoritative and immediately drops state, liveness and the panel (`:66-74`); there is no grace period or second confirmation.
- Fix sketch: Require two consecutive misses (or a miss plus a prompt beacon) before dropping a known agent; never auto-close a pinned panel.
- Evidence: `agentActions.js:59-75, 220-229`.

### [P3] Restored tmux-backed terminals lose ownership: closing them leaves the tmux session running
- Area: `app/frontend/src/lib/actions/terminalActions.js:288`
- Platform: linux-webkitgtk, macos-webkit (tmux hosts)
- Symptom: After a restart, closing a rehydrated bash/zsh pane does not kill its `mimir-…` tmux session; `tmux -L mimir ls` accumulates detached sessions over time.
- Repro: code reasoning, confidence high (may be intentional; then it needs a visible "detached, not killed" hint)
- Cause: `tmuxOwned` is forced false when `restoring` (`:288, :301`), and `removeTerminal` kills only owned sessions (`:556-558`).
- Fix sketch: Persist ownership in the session state and restore it, or offer "close and kill session" for restored panes.
- Evidence: `terminalActions.js:286-301, 554-558`; `App.svelte:380-382`.

### [P3] Update download state can stick and malformed progress events vanish silently
- Area: `app/frontend/src/lib/actions/updateActions.js:129`
- Platform: all
- Symptom: If the backend neither emits `done` nor `error` (process killed mid-download, payload not JSON) the Settings card keeps "downloading" forever with the button disabled; the `catch (_) {}` around `JSON.parse` hides the reason.
- Repro: code reasoning, confidence medium
- Cause: `updateDownloading` is only reset by the two terminal stages (`App.svelte:486-493`); parse failures are swallowed (`:494`).
- Fix sketch: Log/surface parse failures; add a timeout or a backend status poll to recover the flag.
- Evidence: `updateActions.js:129-145`; `App.svelte:483-495`; `app/app_update.go:87`.

## Not found / verified OK

- `agentWorkspaceStore.workspaceCall` serialises reads and saves; project/task/link saves are not optimistic and failures stay visible in `workspaceError` — OK.
- Claude hook install (Settings and panel hint) re-reads status after the call, has a busy flag and shows errors — OK.
- Session name / project name annotations are awaited with error feedback — OK (only the debounced note is affected, see P1 above).
- Listener cleanup on normal close: `cleanupHandlers` covers output/closed/prompt/disconnected subscriptions, link provider, resize observer, paste handler and the agent watch; `App.onDestroy` removes window listeners and `update-progress` — OK.
- Events for closed ids: terminal handlers are unsubscribed in `finalizeTerminalRemoval`; `agent-state-<id>` for an unknown id is dropped (`handleAgentStateEvent` `if (!current) return`) — OK.
- Double click on ✕: the second `removeTerminal` finds nothing after the first finalize — OK.
- `terminal-disconnected` during/after reconnect: the backend emits it only from the reader goroutine of the session that ended; `reconnecting` is reset in both the success and failure branch — OK.
- Restore: per-terminal `try/catch` with rate-limit retry exists for local terminals, saved entries are de-duplicated by `resumeId`, minimized terminals subscribe their events without a DOM element — OK.
- Svelte legacy `$:` chains in `AgentTranscriptPanel.svelte`: no `effect_update_depth_exceeded` or page errors during the panel repros; `clock`, `procsTimer`, `refreshTimer`, `noteTimer` are cleared in `onDestroy` — OK.
- Hypothesis "first `agent-state` event emitted by the backend watcher before the frontend subscribes is lost" (`app_agents_watch.go` emits on the first loop; `subscribeState` runs after the `DetectAgentForTerminalJSON` promise resolves): plausible from ordering but not reproducible with the mocked bridge; left unconfirmed.
