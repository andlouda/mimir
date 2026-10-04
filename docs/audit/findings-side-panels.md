# Findings — side-panels

Scope: `MarkdownNotes.svelte`, `FileBrowser.svelte`, `HistoryDashboard.svelte`,
`RecordingPlayer.svelte` (+ `AggDownloadModal`), `ActivityLogViewer.svelte`,
`AgentTranscriptPanel.svelte`, `views/AgentWorkspaceView.svelte`.
Live repro via the Playwright fixture (`installMimirMocks` plus per-test
overrides of the note/file/history/log/recording/agent bindings; production
build on 127.0.0.1:4173, Chromium). Backend behaviour that the mocks cannot
show (SFTP latency, real cast files, agent session files) is marked as code
reasoning. 20 findings: 5 P1, 13 P2, 2 P3.

### [P1] Notes: "Back" on an unsaved note resurrects a nameless ghost editor and saves to filename `null`
- Area: `app/frontend/src/lib/MarkdownNotes.svelte:188` (`goBackToList`) with `:74`
- Platform: all
- Symptom: Edit a note, press ← without saving. The list appears for ~400 ms, then an empty editor with a blank title comes back. Typing there and pressing Save calls `SaveNote(null, …)`; pressing ← again repeats the cycle once more.
- Repro: 1. Ctrl+Shift+N, open `todo.md`. 2. Type anything. 3. Click ←. 4. Wait for the save round trip. Observed: `.note-editor` visible, `.notes-title` = "", textarea empty; `SaveNote` log: `["todo.md","changed body"]`, then `[null,"typed into ghost"]`.
- Cause: `goBackToList` fires `saveCurrentNote()` without awaiting and sets `activeNote = null` synchronously. After `await SaveNote(...)` resolves, `saveCurrentNote` runs `activeNote = { ...activeNote, content: editorContent }` (line 74) with `activeNote === null`, producing `{content}` — truthy, no `filename` — which re-renders the editor branch. The backend `notes.Store.Save` would then receive filename `"null"`/empty.
- Fix sketch: Await the save before clearing `activeNote` (or capture the note in a local and only write back if `activeNote` still is that note); disable ← while a save is in flight.
- Evidence: `MarkdownNotes.svelte:70-81`, `:188-195`; `docs/audit/shots/side-panels-notes-ghost.png`.

### [P1] Notes: closing the panel with unsaved edits discards them silently
- Area: `app/frontend/src/lib/MarkdownNotes.svelte:245` (`dispatch('close')`)
- Platform: all
- Symptom: Type in a note, click × (or Ctrl+Shift+N). The panel closes; reopening shows the old content. No prompt, no autosave, no "unsaved" warning — only the small `*` that was visible before closing.
- Repro: 1. Open `todo.md`, type "unsaved edit before close". 2. Click `.notes-close-btn`. 3. `GetNote('todo.md')` still returns the original content; `SaveNote` was never called (`savesAfterClose: []`).
- Cause: The close button dispatches `close` directly; `TerminalsPage` unmounts `MarkdownNotes` (`{#if notesPanelOpen}`), and nothing in the component checks `dirty` on destroy or before `close`. `openNote` and `goBackToList` do autosave, so users learn to expect it.
- Fix sketch: In the close handler (and on destroy) save when `dirty && activeNote`, or confirm; make the global Ctrl+Shift+N path go through the same guard.
- Evidence: `MarkdownNotes.svelte:245`, `:36`, `:188-195`; `views/TerminalsPage.svelte:143-155`.

### [P1] File browser: out-of-order directory responses show the wrong listing under the current path
- Area: `app/frontend/src/lib/FileBrowser.svelte:75` (`loadDirectory`)
- Platform: all (most likely over SFTP)
- Symptom: Enter a slow directory, go back up, enter a fast one. When the slow response arrives it replaces the listing while the path bar still shows the fast directory. Every row action (`View`, `Insert`, `Open terminal here`) now joins the *current* path with a file name from the *other* directory.
- Repro: 1. Files page; mock `/slow` answers after 1.5 s. 2. Click `slow`, then `Up`, then `fast`; `y.txt` is shown. 3. Wait 1.8 s. Observed: path bar `/home/user/projects/fast`, list `["📄 x.txt"]` (content of `/slow`).
- Cause: `loadDirectory` sets `currentPath` before the await and assigns `files` after it with no request token/sequence check; `navigateTo` does not cancel in-flight calls. Row actions compute `joinPaths(currentPath, file.name)` from the two now-inconsistent states.
- Fix sketch: Keep a request counter (as `AgentWorkspaceView.listSessions` already does) and drop responses whose token is stale; store the path together with the listing.
- Evidence: `FileBrowser.svelte:75-94`, `:96-98`, `:225`, `:234-240`.

### [P1] Activity log: two entries with the same kind, second and title blank the whole list
- Area: `app/frontend/src/lib/ActivityLogViewer.svelte:228` (keyed `{#each}`)
- Platform: all
- Symptom: The Logs page shows "0 entries — No log entries match the current filter" although the backend returned entries; no error banner. Console: `[mimir] unhandled error: Uncaught Error: https://svelte.dev/e/each_key_duplicate`.
- Repro: 1. Mock `GetActivityLogsJSON` with two `tool_executions` entries sharing `timestamp` and `title` among 500 others. 2. Open Audit → Logs. Observed: `.log-card` count 0, meta "0 entries", page error as above. Removing the pair restores 500 cards.
- Cause: The each key is `${kind}-${timestamp}-${title}`. Backend timestamps are `time.Now().Format(time.RFC3339)` (second precision, e.g. `app/app.go:509`, `app/ai.go:435`), and titles are derived from tool/mode names, so two tool executions in the same second collide. Svelte 5 throws `each_key_duplicate` in the production build too, which aborts the render of the list.
- Fix sketch: Use the array index or a counter appended to the key (or have the backend emit a stable id/nanosecond timestamp); wrap list rendering so one bad entry cannot blank the page.
- Evidence: `ActivityLogViewer.svelte:228`, `:232`; `app/app.go:509`; `docs/audit/shots/side-panels-activitylog-dupkey-blank.png`.

### [P1] Recordings: GIF/scrubbed export shows no progress, allows double start, and reports its result only on the Terminals page
- Area: `app/frontend/src/lib/RecordingPlayer.svelte:280` (`handleExportGif`) and `lib/AppMainContent.svelte:230`
- Platform: all
- Symptom: Clicking "Export GIF" gives no visible reaction for the whole agg run; a second click starts a second export. The success path (or failure) is never shown on the Recordings page — it appears later as a red error banner on the Terminals page ("GIF export saved: /tmp/out.gif").
- Repro: 1. Audit → Recordings, select a recording. 2. Click "Export GIF" twice (mock takes 1.5 s). Observed: label stays "Export GIF", `disabled` false, `ExportRecordingGIF` called 2×, body never contains the path; after switching to Terminals the `.error-message` banner reads "GIF export saved: /tmp/out.gif".
- Cause: `exportingGif = true` and `finally { exportingGif = false }` wrap a synchronous `dispatch`, so the flag is reset in the same tick. The parent awaits the binding and pushes both success and failure through `onError(...)` → `errorMessage`, which is only rendered inside `TerminalsPage.svelte:93-96`. No `update-progress`-style event exists for agg.
- Fix sketch: Let the parent return a promise (or pass busy state back) so the button stays disabled until done; show results in the Recordings page (toast or inline status) and keep the error banner for errors only; emit progress from `ExportRecordingGIF`.
- Evidence: `RecordingPlayer.svelte:280-288`, `:316-334`, `:463-475`; `AppMainContent.svelte:224-248`; `views/TerminalsPage.svelte:93-96`; `docs/audit/shots/side-panels-gif-result-banner.png`.

### [P2] Agent panel: auto-refresh never fires while the agent is working
- Area: `app/frontend/src/lib/AgentTranscriptPanel.svelte:169` (`$: scheduleAutoRefresh(agent?.status)`)
- Platform: all
- Symptom: With the agent in `working`, the snippets/commands/files tabs stay frozen until the agent goes idle or the user presses ↻, although the code promises a 6 s refresh.
- Repro: 1. Open the agent panel for a detected agent. 2. Emit `agent-state-1` `{state:'working', activity:…}` every 2 s for 14 s. Observed: 0 calls to `GetAgentTranscriptJSON` during that time (expected 2).
- Cause: `setState` in `agentActions.js:30-41` creates a new `agent` object on every event (activity text, `activityAt`), so the `$:` statement re-runs each time and `scheduleAutoRefresh` clears and restarts the interval before it can elapse. Claude Code emits activity updates every few seconds while working.
- Fix sketch: Key the reactive statement on the status string only (`$: status = agent?.status; $: scheduleAutoRefresh(status)`) or compare against the previous status before resetting the timer.
- Evidence: `AgentTranscriptPanel.svelte:169`, `:243-246`; `actions/agentActions.js:30-41`.

### [P2] Agent panel: "To notes" twice within a second overwrites the first note
- Area: `app/frontend/src/lib/AgentTranscriptPanel.svelte:345` (`saveToNotes`)
- Platform: all
- Symptom: Saving two snippets quickly reports "Saved as agent-claude-…-13.md." twice but only one note exists; the first snippet is gone.
- Repro: 1. Agent panel with two code snippets. 2. Click "To notes" on both within the same second. Observed: `SaveNote` called twice with the identical filename `agent-claude-2026-10-04T16-01-13.md`; notes list shows one new note.
- Cause: The filename stamp is `toISOString().slice(0, 19)` (second resolution) and `SaveNote` overwrites. The same applies to `saveTextToNotes` (`:331`).
- Fix sketch: Add milliseconds/counter or check `ListNotes` for a collision and suffix `-2`; or append to a per-session note instead of a file per snippet.
- Evidence: `AgentTranscriptPanel.svelte:331-354`.

### [P2] File browser: no loading state — the old listing stays under the new path
- Area: `app/frontend/src/lib/FileBrowser.svelte:78`
- Platform: all (SFTP latency makes it visible)
- Symptom: Clicking a directory immediately changes the path bar, but the previous directory's rows remain clickable for as long as the request takes; no spinner or "Loading…" text.
- Repro: 1. Files page, mock `/slow` with 1.5 s delay. 2. Click `slow`. Observed: path `/home/user/projects/slow`, still 5 rows from the parent, no text matching /load/i.
- Cause: `currentPath = normalizePath(path)` is set before `await ListDirectory(...)`; `files` is only replaced afterwards and there is no `loading` flag.
- Fix sketch: Add a `loading` flag that dims the list and disables row actions, or clear `files` when the path changes (combined with the request token from the P1 above).
- Evidence: `FileBrowser.svelte:75-94`, `:220-249`.

### [P2] File browser: long file names push the action buttons off-screen
- Area: `app/frontend/src/lib/FileBrowser.svelte:398` (`.file-name`)
- Platform: all
- Symptom: A 160-character name makes the row wider than the pane; `View`/`Insert`/`Explorer` for that row sit beyond the window edge and the list scrolls horizontally.
- Repro: 1. Files page at 1100 px with a file `aaaa…(160).txt`. Observed: `ul.file-list` scrollWidth 1316 vs clientWidth 880; first action button at x = 1372 (window 1100).
- Cause: `.file-name` has `flex-grow: 1; white-space: nowrap` but no `min-width: 0; overflow: hidden; text-overflow: ellipsis`, so the flex item grows to its content.
- Fix sketch: Ellipsize `.file-name` with `min-width: 0` and a `title` attribute; keep `.file-actions` with `flex-shrink: 0`.
- Evidence: `FileBrowser.svelte:335-340`, `:398`, `:223-231`.

### [P2] File browser: Escape does not close the file-content modal
- Area: `app/frontend/src/lib/FileBrowser.svelte:252`
- Platform: all
- Symptom: After opening a file preview, pressing Escape does nothing; the modal only closes via the Close button or clicking the backdrop.
- Repro: 1. Click `README.md`. 2. Press Escape. Observed: `.modal-content` still visible; `document.activeElement` is still the `.file-name` span.
- Cause: The `keydown` handler lives on the overlay (`tabindex="0"`) but focus is never moved there; `.modal-content` additionally stops `keydown` propagation. `AggDownloadModal` shares the pattern.
- Fix sketch: Register a window `keydown` listener while the modal is open (or focus the dialog on mount and handle Escape on it), as other modals in `lib/modals` do.
- Evidence: `FileBrowser.svelte:252-253`; `modals/AggDownloadModal.svelte:10-11`.

### [P2] History: backend errors are shown as "No commands found"
- Area: `app/frontend/src/lib/HistoryDashboard.svelte:58`
- Platform: all
- Symptom: When `SearchCommandHistory` fails (locked DB, missing table) the list shows the empty state and the counter "0 commands"; the error only reaches the console.
- Repro: 1. Audit → History, mock throws for query `boom`. 2. Type `boom`. Observed: `.history-list` text "No commands found", no `.error-message`.
- Cause: Both `result.error` and the `catch` branch set `entries = []` and `console.error(...)` only; no error state exists in the template.
- Fix sketch: Keep an `errorMessage` like the other audit pages and render it above the list; distinguish "no match" from "could not load".
- Evidence: `HistoryDashboard.svelte:42-72`, `:225-228`.

### [P2] History consent banner: "Not now" comes back after every restart
- Area: `app/frontend/src/App.svelte:615` with `lib/stores/uiStore.js:60`
- Platform: all
- Symptom: Users who decline command-history tracking see the banner again on every launch until they click "Enable".
- Repro: 1. Start with tracking disabled; banner visible. 2. Click "Not now" → hidden. 3. Reload. Observed: banner visible again.
- Cause: `historyConsentDismissed` is a plain `writable(false)` with no persistence, while the "Enable" path persists through `SetHistoryTracking(true)`.
- Fix sketch: Persist the dismissal (localStorage or a backend setting) and offer the toggle in Settings → History instead of re-asking; optionally re-ask after an update.
- Evidence: `App.svelte:610-615`; `uiStore.js:60`.

### [P2] Recordings: a failed load keeps the previous recording's frames under the new title
- Area: `app/frontend/src/lib/RecordingPlayer.svelte:91`
- Platform: all
- Symptom: Selecting a recording whose file is missing/corrupt highlights it, shows no error, and ▶ plays the previously loaded recording.
- Repro: 1. Select `First recording`, play. 2. Select `Second (broken)` (mock `GetRecording` throws). Observed: `.recording-item.selected` = Second, zero `.error-text`, ▶ renders "hello-rec1 line two" from the first cast.
- Cause: `selectedId` is set before `await loadRecording`, and the `catch` just `return`s; `frames`, `header`, `totalDuration` keep the old values.
- Fix sketch: Clear `frames`/`header` and show an inline error in the player area on failure; keep `selectedId` on the failed item so the error is attributable.
- Evidence: `RecordingPlayer.svelte:86-118`.

### [P2] Notes: deleting a note has no confirmation and no undo
- Area: `app/frontend/src/lib/MarkdownNotes.svelte:317`
- Platform: all
- Symptom: The small × on a list row deletes the file immediately; a slip while aiming for the rename pencil next to it loses the note.
- Repro: 1. Hover `big.md`, click `.note-delete-btn`. Observed: no dialog event, `DeleteNote('big.md')` called at once.
- Cause: `deleteNote` calls the binding directly; the two 1-character buttons sit side by side (`:315-318`).
- Fix sketch: Confirm (inline "Delete? Yes/No" on the row) or move the file to a trash folder with an undo toast.
- Evidence: `MarkdownNotes.svelte:101-113`, `:315-318`.

### [P2] Notes preview: GFM task-list checkboxes are stripped
- Area: `app/frontend/src/lib/util.js:19` (`SANITIZE_ALLOWED_TAGS`)
- Platform: all
- Symptom: `- [ ] write audit` / `- [x] read code` render as plain bullets "write audit" / "read code"; checked and unchecked items look identical. The agent panel's context-summary prose uses the same sanitizer.
- Repro: 1. Open `todo.md`, click Preview. Observed: 0 `input` elements, HTML `<li> write audit</li><li> read code</li>`.
- Cause: marked (GFM on by default) emits `<input type="checkbox" disabled checked>`; `input` is not in `SANITIZE_ALLOWED_TAGS`, and `type`/`checked`/`disabled` are not in `SANITIZE_ALLOWED_ATTRS`, so DOMPurify drops the element and leaves the leading space.
- Fix sketch: Allow `input` with `type="checkbox"`, `checked`, `disabled` only (DOMPurify hook to force `disabled`), or post-process task items into `☐/☑` text.
- Evidence: `util.js:19-25`; `MarkdownNotes.svelte:220`.

### [P2] Side panels: notes width (≤ 800 px) and agent panel (440 px) are not viewport-relative — terminals shrink to 0 px
- Area: `app/frontend/src/App.svelte:261` (`startNotesDrag` clamp) with `src/styles/main.css:199-210`
- Platform: all
- Symptom: On a 1100 px window with the agent panel open and a wide notes panel (persisted from a larger monitor or set by dragging), the terminal area collapses completely; nothing hints that terminals are still there.
- Repro: 1. `mimir-notes-width=800`, window 1100 px. 2. Open the agent panel, then "To notes" (opens Notes). Observed: sidebar 220 + agent 440 + notes 800 = 1460 > 1100; `.terminal-area` width 0.
- Cause: The drag clamp is a fixed 250–800 px and the stored width is restored verbatim; `.notes-panel`/`.agent-panel` are `flex-shrink: 0`, so the flex item that gives way is the terminal area.
- Fix sketch: Clamp the restored and dragged width to a share of the window (e.g. ≤ 45 %) and give the terminal area a `min-width`; when both panels are open, cap their sum.
- Evidence: `App.svelte:256-270`; `main.css:195-212`; `uiStore.js:5`.

### [P2] Agent workspace: "Edit task" on a long list opens the editor off-screen
- Area: `app/frontend/src/lib/views/AgentWorkspaceView.svelte:162`
- Platform: all
- Symptom: Scroll to the bottom of 30 tasks, click "Edit task" — nothing visible happens; the form rendered 3400 px above the viewport.
- Repro: 1. Seed 30 tasks, Tasks tab, scroll `.workspace` to bottom. 2. Click the last "Edit task". Observed: `.editor` bounding box y = −3410, `scrollTop` 3555 unchanged.
- Cause: The editor is a single `{#if editor}` block placed above the tabs; `editTask` does not scroll it into view or focus a field.
- Fix sketch: `scrollIntoView` + focus the first input after `await tick()`, or render the editor inline in the row / as a dialog.
- Evidence: `AgentWorkspaceView.svelte:64-68`, `:162-192`.

### [P2] Audit pages lose search, filters, selection and playback on every page switch
- Area: `app/frontend/src/lib/AppMainContent.svelte:107` (`{#if currentPage === …}` chain)
- Platform: all
- Symptom: Type a history search, change the range to "All", go back to terminals to copy something, return — the search is empty and the range is "7d" again. The Logs page re-fetches and re-selects the first entry; a playing recording stops and deselects.
- Repro (history): 1. Audit → History, search `kube`, range "All". 2. ← back, Audit → History. Observed: `.search-input` "", range "7d". (Logs/Recordings by code reasoning, confidence high — same unmount.)
- Cause: Each page component is created and destroyed by the `{#if}` chain; none of them lifts its state into a store (`uiStore` holds only `currentPage`).
- Fix sketch: Keep lightweight filter/selection state in a store per page, or keep audit components mounted but hidden (`display: none`) as is already done for terminals.
- Evidence: `AppMainContent.svelte:107-248`; `HistoryDashboard.svelte:10-23`; `ActivityLogViewer.svelte:15-23`; `RecordingPlayer.svelte:13-16`.

### [P3] Agent panel: note draft and "note open" state carry over when the panel switches to another agent
- Area: `app/frontend/src/lib/AgentTranscriptPanel.svelte:65` (`$: if (!noteOpen) noteDraft = …`) with `:185`
- Platform: all
- Symptom: With the note textarea open for agent A, clicking agent B's badge keeps the textarea open and still showing A's draft; typing there saves A's text into B's annotation, and a pending 600 ms autosave started under A fires with `terminalId` already pointing to B.
- Repro: code reasoning, confidence medium (the fixture's agent has no `sessionFile`, so the Note button is disabled there; verified `disabled` in the mock run).
- Cause: `resetFor()` resets transcript/view/sessions but not `noteOpen`, `noteDraft`, `editingName`, `editingProject`; `noteDraft` only re-syncs when `noteOpen` is false; `noteChanged`'s timeout reads `terminalId`/`note` at fire time.
- Fix sketch: In `resetFor()` close the editors, clear the drafts and cancel `noteTimer` (flush it for the previous terminal first); capture `terminalId` in `noteChanged`.
- Evidence: `AgentTranscriptPanel.svelte:56-82`, `:150`, `:185-188`, `:404-411`.

### [P3] Side panels: hard-coded English and mixed-locale strings
- Area: `app/frontend/src/lib/FileBrowser.svelte:91` (one of several)
- Platform: all
- Symptom: With the DE locale the file browser errors ("Failed to load directory: …"), notes errors ("Failed to save: …") and relative times ("just now", "5m ago" in Notes and Recordings) stay English; the history list mixes "3h ago" with a hard-coded `de-DE` date ("04.09.26") even in EN; Activity log kind labels ("Tools", "Security"), detail labels and "JSON copied." bypass `$t`; agent panel "Copy failed:"/"Save failed:" likewise.
- Repro: 1. History at 1600 px: `.time-tag` of a 30-day-old row reads `04.09.26` while newer rows read `5m ago`. Others by code reasoning, confidence high.
- Cause: Template literals instead of `$t(...)`; `toLocaleDateString('de-DE', …)` with a fixed locale; `formatTimeAgo` duplicated in three components.
- Fix sketch: Route the strings through `i18n.js`, share one `formatTimeAgo` using the active locale (`Intl.RelativeTimeFormat`), drop the fixed `de-DE`.
- Evidence: `FileBrowser.svelte:91,114,137,154,184`; `MarkdownNotes.svelte:48,66,79,97,111,137,151,166,197-203`; `HistoryDashboard.svelte:117-135`; `ActivityLogViewer.svelte:25-53,83-85,101-119`; `RecordingPlayer.svelte:256-262`; `AgentTranscriptPanel.svelte:314,340,353,365`.

## Not found / verified OK

- Activity log search is not debounced but cheap: 5 keystrokes over 500 entries with 8 kB payloads each took 14–17 ms per keystroke (JSON.stringify per entry per keystroke, `ActivityLogViewer.svelte:55-62`). Fine up to the 500 limit.
- Activity log at 900 × 700 collapses to one column (`@media` at `:626`) and keeps the detail pane reachable (list 121 px high, detail below, root not clipped). Cramped but workable; long titles overflow the card horizontally (card scrollWidth 2715 vs 602 px, `:239`) — polish only.
- Agent panel timers: `onDestroy` clears note, procs, clock, refresh, refreshSoon and feedback timers (`AgentTranscriptPanel.svelte:404-411`); the `processes` tab interval starts/stops with the tab (`:91`). No leak found. `view = 'snippets'` self-assignments (`:118,143,144`) did not raise `effect_update_depth_exceeded` in the mock run.
- Agent panel feedback (`flash`, 1.8 s) shows "Saved as …" / "Copied" reliably; "Insert" uses `xterm.paste` (bracketed paste), nothing executes on its own.
- HistoryDashboard `searchTimeout` is not cleared on destroy but only triggers a harmless state write on an unmounted component; mount issues two identical searches (`onMount` + the `filterKey` reactive once `mounted` flips, `:153-162`) — minor extra IPC.
- History row truncation: `truncateCmd(60)` cuts commands at a fixed 60 chars even on a 1600 px window (text 63 chars, no CSS overflow) — space is wasted but nothing is hidden twice.
- RecordingPlayer: `rafId` cancelled and `term.dispose()` on destroy. `fitAddon.fit()` right after `term.resize(header.width, header.height)` overrides the recorded geometry (35 rows rendered for a 24-row cast), so wrapping can differ from the recording — polish. Seeking replays every frame from 0 per slider `input` event (`:214-237`); fine for short casts, may stutter on long ones (code reasoning, confidence medium).
- Notes: textarea scroll position (3000 px) resets to 0 after Edit → Preview → Edit because the textarea is re-created (`MarkdownNotes.svelte:340-361`) — polish.
- Notes rename: Enter + blur call `confirmRename` twice but the second call exits on `renameTarget === null`; no double rename observed.
- FileBrowser: empty directories show an empty list without a message (`:220-249`) — polish; `Up` is correctly disabled at `/` and `C:/`; no upload code exists in the component despite the surface map's "read/upload" note, so no upload-progress issues apply.
- AgentWorkspaceView: `listSessions` uses a request token (stale responses dropped), `request++` on destroy; the editor `fieldset` disables during `busy`; unlink has no confirm but is reversible by re-linking. Project chips clip long names without an ellipsis (`display: inline-flex` defeats `text-overflow`, `:309`; 586 px content in 318 px) — polish.
- Hook hint: dismissed-set is module-level and survives panel re-opens within the app session as intended; pin toggles `agentPanelPinned` with `aria-pressed`.
- i18n keys: all 275 `$t(...)` keys used by the seven components exist in both `en.js` and `de.js`.
