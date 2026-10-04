# Findings — forms-modals

Scope: `lib/modals/*.svelte`, `workflows/WorkflowApprovalDialog.svelte`,
`SecretUnlockGate.svelte`, the update panel in `views/SettingsView.svelte`
(`updateStore`/`updateActions`), the editor forms in
`views/AgentWorkspaceView.svelte`, the annotation inputs in
`AgentTranscriptPanel.svelte`. Paths below are relative to `app/frontend/src`.

Live reproduction: throw-away spec `tests/e2e/zz-audit-forms-modals.spec.js`
(deleted) on top of `installMimirMocks`. The fixture lacks `SecretStoreState`,
`AskAI`, `ApplyTemplateWithVariables`, `SetAgentSessionAnnotation`,
`DeleteSSHProfile`, `SaveSSHProfile`, `StartSSHTerminal` (host-key path) and
returns no SSH profiles/templates; the spec stubbed these with recording
mocks. Chromium only (Playwright); WebKitGTK/WebView2 behaviour is inferred
where noted.

---

### [P1] Master password can be typed into the shell while the secret gate is shown
- Area: `SecretUnlockGate.svelte:456` (overlay) with `App.svelte:509` and `lib/actions/terminalActions.js:219`
- Platform: all
- Symptom: On a start with a locked store the gate card is shown but keyboard focus stays in the terminal underneath. A user who starts typing the master password sends it to the shell (and so into scrollback, history, recordings, transcripts). Tab from the password field leaves the gate and lands on the sidebar.
- Repro: 1. Stub `SecretStoreState → 'locked'`. 2. Load the app, wait for `#terminal-1`. 3. `document.activeElement` is `textarea.xterm-helper-textarea`. 4. Type `hunter2` → `WriteToTerminal(1, 'h')`, `(1,'u')`, … recorded. 5. Click the password field, press Tab four times → focus on `div.sidebar-heading` outside the gate. (verified)
- Cause: `App.svelte:509` mounts `<SecretUnlockGate />` without `onresolved`, so the session restore in `onMount` (`App.svelte:427ff`) creates terminals and `terminalActions.js:219` calls `terminal.focus()` regardless of the gate. The gate's inputs have no autofocus and the overlay has no focus trap / `inert` on the rest of the app.
- Fix sketch: Autofocus the password input when the phase becomes setup/unlock; mark `<main>` `inert` (or trap Tab) while the gate is up; defer terminal focus/restore until `onresolved`.
- Evidence: `SecretUnlockGate.svelte:455-508`, `App.svelte:509`, `terminalActions.js:216-221`

### [P1] "Insert into Terminal" executes the AI-generated command immediately
- Area: `lib/modals/AIPanelModal.svelte:71`
- Platform: all
- Symptom: The secondary button labelled "Insert into Terminal" writes the suggested command *plus a carriage return* to the PTY, i.e. it runs the command without a chance to edit or confirm. The review note (`aiPanel.reviewNote`, "not inserted automatically") only applies when the backend flags a warning.
- Repro: 1. Stub `AskAI → {"text":"rm -rf /tmp/scratch"}`. 2. Terminal controls → AI → "Write Command From Goal". 3. Enter a goal, Run. 4. Click "Insert into Terminal" → `WriteToTerminal(1, "rm -rf /tmp/scratch\r")` recorded; the panel stays open so a second click runs it again. (verified)
- Cause: `insert()` appends `'\r'` to `state.result` (`AIPanelModal.svelte:71`); no confirmation, no "insert without executing" option, no disabled state after insert.
- Fix sketch: Write the text without `\r` (let the user press Enter), or rename the button to "Run" and require an explicit confirm for medium/high-risk output; disable after one insert.
- Evidence: `AIPanelModal.svelte:66-75`, `AIPanelModal.svelte:122`, `locales/en.js:222-223`

### [P1] Approval dialog can be dismissed and never reopened; workflow stays "Waiting for approval"
- Area: `workflows/WorkflowApprovalDialog.svelte:485` and `workflows/WorkflowBuilder.svelte:890`
- Platform: all
- Symptom: A click on the dark backdrop (or on "Close") hides the approval dialog while the run is still `pending_approval`. The summary shows "Waiting for approval" and an "Approval Needed" card, but no control brings the dialog back; the only way forward is to re-run the whole playbook.
- Repro: 1. Workflow → "Approval Drill" → Run. 2. Click the overlay at (4,4) → `.approval-modal` gone. 3. No button matching /approve|continue|resume/ remains on the page; status card still says "Waiting for approval". (verified; `docs/audit/shots/forms-modals-approval-closed.png`)
- Cause: `close={() => { showApprovalDialog = false; }}` (`WorkflowBuilder.svelte:890`); `showApprovalDialog` is only set to true inside the run/approve handlers (`:446,:472,:501`). The overlay's `on:click` closes on any outside click (`WorkflowApprovalDialog.svelte:485`).
- Fix sketch: Don't close on backdrop click for a pending approval (or treat close as "decide later" and add a "Review approval" button on the Approval Needed card that sets `showApprovalDialog = true`).
- Evidence: `WorkflowApprovalDialog.svelte:481-494`, `WorkflowBuilder.svelte:884-893`

### [P1] Template prompt has no double-submit guard: Enter twice runs the template twice
- Area: `lib/modals/TemplatePromptModal.svelte:232` with `lib/actions/templateActions.js:215`
- Platform: all
- Symptom: Pressing Enter (or clicking Run Template) a second time while the first apply is in flight sends the command to the terminal twice. For workflow prompts (`state.kind === 'workflow'`) the same applies to `RunWorkflowDraftJSON`.
- Repro: 1. Stub a template `ping -c 3 {{.Host}}` and a 400 ms `ApplyTemplateWithVariables`. 2. Ctrl+Shift+P → Enter → fill Host. 3. Press Enter twice → `ApplyTemplateWithVariables` recorded 2×. (verified)
- Cause: `submitTemplatePrompt` has no in-flight flag; the modal stays open until the promise resolves (`templateActions.js:215-245`) and the Run button is never disabled (`TemplatePromptModal.svelte:243`).
- Fix sketch: Add `submitting` to `templatePromptState`, disable Run/inputs and ignore Enter while set; close optimistically or show "Running…".
- Evidence: `TemplatePromptModal.svelte:214,232,243`, `templateActions.js:215-245`

### [P2] Escape never closes seven modals once focus is inside them
- Area: `lib/modals/TemplatePromptModal.svelte:199` (same pattern in `AIPanelModal.svelte:91`, `AISettingsModal.svelte:168`, `FunctionCatalogModal.svelte:500`, `AggDownloadModal.svelte:347`, `DotEnvViewerModal.svelte:123`, `workflows/WorkflowApprovalDialog.svelte:488`)
- Platform: all
- Symptom: With the cursor in any field or on any button of these dialogs, Escape does nothing. The global window handler only closes the template/workflow pickers.
- Repro: 1. Open the template prompt, click into the Host input, press Escape → still open. 2. AI panel: click into the goal textarea, Escape → still open. 3. Approval dialog: Escape → still open. (verified)
- Cause: The Escape handler sits on the overlay (`on:keydown` of `.modal-overlay`), but the dialog element has `on:keydown|stopPropagation`, so key events from inside never reach it. `keyboardShortcuts.js:197-205` only handles the pickers.
- Fix sketch: Move the Escape handler onto the dialog element (or `<svelte:window on:keydown>` scoped by an `open` flag) and drop the inner `stopPropagation`.
- Evidence: `TemplatePromptModal.svelte:186-200`, `AIPanelModal.svelte:78-92`, `keyboardShortcuts.js:197-205`

### [P2] Modals open without initial focus and without a focus trap
- Area: `lib/modals/SSHProfileModal.svelte:315` (same in `HostKeyModal.svelte:498`, `TemplatePromptModal.svelte:186`, `AIPanelModal.svelte:78`, `AISettingsModal.svelte:155`, `FunctionCatalogModal.svelte:499`, `AggDownloadModal.svelte:346`, `DotEnvViewerModal.svelte:122`, `WorkflowApprovalDialog.svelte:481`)
- Platform: all
- Symptom: After opening, `document.activeElement` is the trigger button or `<body>`; Escape does nothing until the user clicks inside (SSH, host-key), Tab walks through the sidebar behind the backdrop, and screen readers are not moved into the dialog. For the host-key prompt there is no keyboard way to reject without a mouse click first.
- Repro: 1. Sidebar "+" (Manage SSH Profiles) → active element `button.sidebar-add-btn`; Escape → modal still open; click the header, Escape → closes. 2. Connect → host-key modal: active element `<body>`, Escape → still open. 3. Template prompt / AI panel / approval: active element `<body>`. (verified)
- Cause: Only `TemplatePicker`, `WorkflowPicker` (`inputEl.focus()` in `onMount`) and `TranscriptViewerModal` (`modalEl.focus()` + Tab loop, `:360-409,:430`) manage focus. All other overlays rely on `tabindex="0" role="button"` on the backdrop, which is never focused programmatically.
- Fix sketch: Extract the focus-trap/initial-focus logic from `TranscriptViewerModal` into a shared action (`use:modalFocus`) and apply it to every overlay; focus the first field (or the safe button for host-key).
- Evidence: `TranscriptViewerModal.svelte:360-436` (reference implementation), `SSHProfileModal.svelte:315`, `HostKeyModal.svelte:498`

### [P2] A drag that ends on the backdrop closes the modal and discards the form
- Area: `lib/modals/SSHProfileModal.svelte:315` (same mechanism in `AISettingsModal.svelte:157`, `TemplatePromptModal.svelte:188`, `AIPanelModal.svelte:80`, `FunctionCatalogModal.svelte:499`, `DotEnvViewerModal.svelte:122`, `AggDownloadModal.svelte:346`, `HostKeyModal.svelte:498` → Reject)
- Platform: all
- Symptom: Selecting text in an input with the mouse and releasing outside the dialog closes it; every typed value (including SSH password and the long AI settings form) is lost without a prompt. On the host-key prompt the same gesture rejects the key.
- Repro: 1. SSH modal → New Profile, type a name and password. 2. mousedown in the Name input, drag to (8,8), mouseup → `.ssh-modal` gone, draft lost. (verified)
- Cause: `click` fires on the nearest common ancestor of mousedown/mouseup targets, which is the overlay itself, so both `on:click|self` and the inner `stopPropagation` variants pass. No unsaved-changes guard exists in any modal.
- Fix sketch: Close on backdrop only when `mousedown` *and* `mouseup` targeted the overlay (track `pointerdown` target); for editors with a dirty form ask before discarding.
- Evidence: `SSHProfileModal.svelte:315`, `AISettingsModal.svelte:155-168`, `HostKeyModal.svelte:498`

### [P2] Errors raised from inside a modal appear behind the backdrop
- Area: `views/TerminalsPage.svelte:93` with `styles/overlays.css:52-61`
- Platform: all
- Symptom: Validation and backend errors from modal actions (template prompt "Please enter a value for Host.", AI panel "Please enter a goal.", function-catalog errors, SSH save failures) are written to the global banner at the top of the terminals page, which is dimmed under the modal's 78 % black overlay; its dismiss "×" cannot be clicked.
- Repro: 1. Ctrl+Shift+P → Enter → Run Template with Host empty. 2. `.error-message` exists at y=49, `elementFromPoint` on its centre returns `.modal-overlay`. (verified; `docs/audit/shots/forms-modals-error-behind-modal.png`)
- Cause: The banner is in normal flow inside the page (`TerminalsPage.svelte:93`); `.modal-overlay` is `position: fixed; z-index: 30` covering the viewport. Modals have no inline error slot (only `SecretUnlockGate`, `AgentWorkspaceView` and `DotEnvViewerModal` render errors in place).
- Fix sketch: Render validation errors inside the dialog (inline `role="alert"`), or portal the global banner above the overlay (z-index > 30, fixed).
- Evidence: `templateActions.js:222`, `AIPanelModal.svelte:36`, `FunctionCatalogModal.svelte:436,450`, `overlays.css:52-61`

### [P2] SSH profile is deleted with one click, no confirmation, same ✕ glyph as "close"
- Area: `lib/modals/SSHProfileModal.svelte:348`
- Platform: all
- Symptom: The ✕ next to each profile deletes it (and its stored password) immediately. It uses the same `header-btn close-btn` style as the modal's close ✕ in the header 30 px above.
- Repro: 1. Open the SSH modal with one profile. 2. Click the row's ✕ → `DeleteSSHProfile('p1')` called immediately, no prompt. (verified)
- Cause: `on:click={() => deleteProfile(profile.id)}` with no confirm step; `TranscriptViewerModal` shows the expected pattern (`pendingDeleteId` + confirm overlay).
- Fix sketch: Inline confirm ("Delete prod?" Cancel / Delete) like the transcript viewer; use a trash glyph and a danger colour instead of ✕.
- Evidence: `SSHProfileModal.svelte:306-312,348`, `TranscriptViewerModal.svelte:252-285`

### [P2] SSH save: password keyed by name+host, and a partial failure leads to duplicate profiles
- Area: `lib/modals/SSHProfileModal.svelte:286`
- Platform: all
- Symptom: (a) When editing, the password is stored on the first profile whose name and host match, not on the edited id; two profiles "prod"/"example.com" with different users get each other's password. (b) If `SaveSSHProfile` succeeds but `SetSSHPassword` throws, the error reads "Failed to save SSH profile", the editor stays in `__new__` mode with the data, and pressing Save again creates a second profile.
- Repro: code reasoning, confidence high
- Cause: `updated.find((p) => p.name === … && p.host === …)` ignores `editingId` (`:286`); the jump-host branch (`:292`) uses the id first, the primary branch doesn't. `editingId`/`form` are only reset after the whole try block (`:299-300`).
- Fix sketch: Resolve the saved id from the backend (return it or match on `editingId` when editing); on password failure switch `editingId` to the saved id and show a password-specific error.
- Evidence: `SSHProfileModal.svelte:277-304`

### [P2] Host-key prompt: the safe choice is nearly invisible, mismatch shows no previous fingerprint
- Area: `lib/modals/HostKeyModal.svelte:525`
- Platform: all
- Symptom: "Reject" is a small text-only `header-btn` next to a filled accent "Trust & Connect" / "Accept Anyway" primary; on a *mismatch* (possible MITM) the only prominent control is the dangerous one. The dialog shows the new fingerprint only, not the stored one, and the backend's fifth payload field (`message`) is parsed but never displayed.
- Repro: Stub `StartSSHTerminal` to throw `HOST_KEY_VERIFY|unknown|example.com|SHA256:…|ssh-ed25519|first contact` and press Connect. (verified; `docs/audit/shots/forms-modals-hostkey-over-profile.png`)
- Cause: Button classes `header-btn` vs `add-btn` (`:525-528`); `parseHostKeyVerifyError` keeps `message` (`sshActions.js:59`) that the modal ignores; the backend payload carries no old fingerprint (`app_ssh.go:176`).
- Fix sketch: Make Reject a normal secondary button and, for mismatch, demote accept to a secondary/danger button with a typed confirmation; render `state.message`; include the stored fingerprint in the mismatch payload.
- Evidence: `HostKeyModal.svelte:504-529`, `lib/actions/sshActions.js:49-63`, `app/app_ssh.go:176`

### [P2] Agent session rename: Escape still saves the draft, Enter saves twice
- Area: `AgentTranscriptPanel.svelte:419` (and `:431` for the project name)
- Platform: windows-webview2 (verified in Chromium; WebKitGTK/macOS likely the same, confidence medium)
- Symptom: Typing a new name and pressing Escape persists the typed text instead of cancelling; pressing Enter sends the annotation twice.
- Repro: 1. Agent panel (badge) → click the session label → type "typed-then-escaped" → Escape → `SetAgentSessionAnnotation(…, 'typed-then-escaped', …)` recorded. 2. Click again, type "entered", Enter → two identical `SetAgentSessionAnnotation` calls. (verified)
- Cause: `on:blur={saveName}` fires when the `{#if editingName}` input is removed from the DOM (Chromium fires blur on removal), both after `editingName = false` on Escape and after `saveName()` on Enter.
- Fix sketch: Keep a `cancelled` flag set on Escape and checked in `saveName`; guard `saveName` with an in-flight/`editingName` check so blur after Enter is a no-op.
- Evidence: `AgentTranscriptPanel.svelte:66-69,419,431`

### [P2] Agent session note: closing the panel within 600 ms drops the last edit
- Area: `AgentTranscriptPanel.svelte:74` with `:405`
- Platform: all
- Symptom: Text typed into the session note and followed by closing the panel (or switching terminals, which re-mounts the panel) is never saved; there is no saving indicator.
- Repro: 1. Agent panel → Note → type "remember this" → click ✕ immediately. 2. Wait 900 ms → no `SetAgentSessionAnnotation` call recorded. (verified)
- Cause: `noteChanged` debounces 600 ms (`:74-78`); `onDestroy` clears `noteTimer` without flushing (`:405`).
- Fix sketch: Flush the pending save in `onDestroy` (and on `blur` of the textarea); show a small "saved" state.
- Evidence: `AgentTranscriptPanel.svelte:72-78,405,448`

### [P2] SSH editor: no required-field validation, no stored-password state, one field for password and passphrase
- Area: `lib/modals/SSHProfileModal.svelte:257`
- Platform: all
- Symptom: Save with empty Name/Host/Username creates a blank profile ("@:22" in the list). When editing, the password field is empty with placeholder "Enter password"; nothing says a password is already stored and there is no way to clear it (leaving it empty silently keeps the old one). Switching Auth Method keeps the typed password and stores it as the key passphrase.
- Repro: code reasoning, confidence high (backend `SaveSSHProfile` only validates JSON, `app_ssh.go:268-276`)
- Cause: `save()` builds `profileData` without checks (`:259-275`); `form.password` is shared by both branches (`:388,:410`); no `hasPassword` flag from the backend.
- Fix sketch: Inline required validation before calling the backend; show "password stored · Clear" like `AISettingsModal` (`:204-211`); separate `password`/`passphrase` form keys.
- Evidence: `SSHProfileModal.svelte:257-304,385-412`, `AISettingsModal.svelte:195-213`

### [P3] "Command History" consent banner floats above every modal
- Area: `App.svelte:612` with `styles/sidebar.css:345-355`
- Platform: all
- Symptom: The bottom-right consent card (z-index 1000) is drawn over the modal backdrop and stays clickable while a modal or the host-key prompt is open; it also hides part of the approval dialog on small windows.
- Repro: visible in both screenshots `docs/audit/shots/forms-modals-hostkey-over-profile.png` and `…-error-behind-modal.png` (verified)
- Cause: `.history-consent-banner { position: fixed; z-index: 1000 }` vs `.modal-overlay { z-index: 30 }`.
- Fix sketch: Give the banner a z-index below the overlays (or hide it while any modal state is set).
- Evidence: `sidebar.css:345-355`, `overlays.css:52-61`

### [P3] Agent workspace editor: inverted button order, no initial focus, Escape ignored, editor out of view
- Area: `views/AgentWorkspaceView.svelte:189`
- Platform: all
- Symptom: The form renders "Save" left and "Cancel" right, the opposite of every modal (Cancel left, primary right). Opening the editor leaves focus on the trigger button; Escape does not cancel; clicking "Edit task" far down a long list opens the editor at the top of the page, out of view.
- Repro: 1. Agent workspace → New project → buttons `["Save","Cancel"]`, active element is the button, type a name, Escape → form still open. (verified)
- Cause: Markup order at `:189`; no `autofocus`/`scrollIntoView` in `editProject`/`editTask` (`:59-68`); no keydown handler on the form.
- Fix sketch: Swap the buttons, focus the first field and `scrollIntoView` the editor when it opens, close on Escape when the form is clean.
- Evidence: `AgentWorkspaceView.svelte:59-68,162-191`

### [P3] agg download dialog can be dismissed mid-download; download continues unseen
- Area: `lib/modals/AggDownloadModal.svelte:346`
- Platform: all
- Symptom: While "Downloading…" is shown, Cancel, ✕, Escape and backdrop still close the dialog; the download keeps running with no progress or completion feedback until the error/empty-state flips.
- Repro: code reasoning, confidence high
- Cause: `onCancel` is not gated on `downloading` (`:346,:350,:368`); `runAggDownload` has no cancellation (`aggActions.js:9-26`).
- Fix sketch: Disable Cancel/✕/backdrop while `downloading`, or keep the dialog as a progress surface until `DownloadAgg` resolves.
- Evidence: `AggDownloadModal.svelte:346-372`, `lib/actions/aggActions.js:9-26`

### [P3] Enter in the AI goal / function question textarea does nothing, and the primary action is misplaced
- Area: `lib/modals/AIPanelModal.svelte:103` and `FunctionCatalogModal.svelte:582,601`
- Platform: all
- Symptom: Enter inserts a newline and there is no Ctrl+Enter submit or hint; the user must reach for the mouse. In the function catalog the filled primary button is "Ask AI" even though the main task is adding functions to a workflow; "Add to Workflow" is a secondary.
- Repro: AI panel: type a goal, press Enter → `AskAI` not called (verified). Catalog: code reasoning, confidence high.
- Cause: No keydown handling on the textareas; button classes at `FunctionCatalogModal.svelte:596-603`.
- Fix sketch: Submit on Ctrl/Cmd+Enter and say so in the placeholder; make "Add to Workflow" the primary.
- Evidence: `AIPanelModal.svelte:100-128`, `FunctionCatalogModal.svelte:580-604`

### [P3] No show/hide toggle on any password field
- Area: `lib/modals/SSHProfileModal.svelte:388` (also `:410,:442,:451`, `AISettingsModal.svelte:198`, `SecretUnlockGate.svelte:465-493`)
- Platform: all
- Symptom: Long passphrases, jump-host passwords, API keys and the master password cannot be checked before saving; a typo in the master-password setup is only caught by the confirm field.
- Repro: code reasoning, confidence high
- Cause: Plain `<input type="password">` everywhere.
- Fix sketch: Shared `PasswordInput` with an eye toggle (default hidden, never persisted).
- Evidence: see Area

### [P3] Workflow picker: Enter while "Loading…" runs the previous list's entry
- Area: `lib/modals/WorkflowPicker.svelte:307` with `lib/actions/templateActions.js:304-313`
- Platform: all
- Symptom: Reopening the picker shows "Loading…" but Enter immediately runs whatever was at `activeIndex` in the last loaded list, which may have been renamed or deleted since.
- Repro: code reasoning, confidence medium
- Cause: `toggleWorkflowPicker` sets `workflowPickerLoading` but keeps `workflowPickerPlaybooks` from the previous open; `onKeydown` does not check `loading`.
- Fix sketch: Clear the playbooks when loading starts, or ignore Enter while `loading`.
- Evidence: `WorkflowPicker.svelte:285-311,323`, `templateActions.js:293-313`

---

## Not found / verified OK

- `TemplatePicker` / `WorkflowPicker`: search input autofocused on mount, Arrow/Enter/Escape handled on the input plus the global window fallback (`keyboardShortcuts.js:197-205`); Enter on an empty list is a no-op. (verified)
- `TranscriptViewerModal`: initial focus on the dialog, Tab/Shift+Tab loop, focus restored to the trigger on close, Escape layering (search → delete confirm → close), stale-load token, delete confirmation with disabled buttons while deleting, document-level pointer close that survives xterm's listeners. Reference implementation for the other modals.
- `DotEnvViewerModal`: consent step before enabling, fixed-width mask (no length leak), `user-select: none` while masked, entries cleared in `onDestroy`, errors shown inline, clipboard through the Wails runtime.
- `AISettingsModal` / `aiSettingsActions.js`: API key is dropped from the store on close and after save, the backend-redacted key never round-trips, stored-key hint with explicit "Clear"; disabled checkboxes for enforced guardrails.
- `SecretUnlockGate`: min-length and mismatch validation, busy/disabled states, backend errors mapped to friendly text, `role="alert"`, password cleared from state on resolve; FIDO button honestly reports "unavailable".
- `AgentWorkspaceView`: `<fieldset disabled={busy}>` blocks double submit; save failure keeps the form (covered by the existing e2e); "Assign" disabled until a session is chosen; stale-token guard for session listing; `required` on project/title/host.
- `SSHProfileModal`: passwords are never rendered in the list, the form is reset after a successful save, the component is destroyed on close so no secret stays in the DOM; Connect buttons are disabled while connecting; the host-key modal stacks correctly above the profile modal (both z-index 30, DOM order in `AppModals.svelte:128-142`).
- `HostKeyModal`: backdrop click and Escape (once focused) map to Reject, i.e. the safe default; fingerprint rendered monospace with `word-break` so SHA256 strings never overflow.
- `WorkflowApprovalDialog`: Deny/Approve both disabled during the request, decision message shown inline, primary on the right.
- Update panel (`SettingsView.svelte:282-327`): Refresh disabled while checking/downloading, progress bar driven by `update-progress`, restart prompt after staging; the `releaseUrl` key used by `openUpdatePage` matches the Go tag (`update/checker.go:33`) — only the Playwright fixture spells it `releaseURL`.
- Button order Cancel-left / primary-right is consistent across `TemplatePromptModal`, `AggDownloadModal`, `DotEnvViewerModal`, `SSHProfileModal` editor, `HostKeyModal`, `WorkflowApprovalDialog`; the only deviations are noted above (`AgentWorkspaceView`, `FunctionCatalogModal`).
- Long modals (`AISettingsModal`, `FunctionCatalogModal`) scroll inside the dialog (`max-height: 88vh; overflow: auto`) and the overlay itself scrolls on short viewports (`overlays.css:61`); not reproduced as a problem.
- `autofocus` in `AgentTranscriptPanel.svelte:418,430` is annotated with `svelte-ignore a11y_autofocus` and limited to inline rename inputs; acceptable.
