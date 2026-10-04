# Findings — i18n-copy

Scope: `app/frontend/src/lib/i18n.js`, `lib/locales/{en,de}.js` (876 lines, 26 blocks,
818 distinct literal keys used in 106 files), hardcoded strings in components,
plural/placeholder handling, date/number formatting, tone and terminology.

Method: a node script (scratchpad) flattened both locale objects and diffed them
against every `$t('…')` / `$tr('…')` / `translate('…')` literal and every
template-literal key prefix in `src/**/*.{svelte,js}`; greps for text nodes,
`title=`, `aria-label=`, `placeholder=` literals and `errorMessage.set(...)`;
one throw-away Playwright spec (deleted) that loaded the app in EN with a
restored saved session and in DE with a Claude agent in `permission` state,
walked terminals / agent panel / Settings / Agent workspace and scanned every
`nowrap`/`overflow:hidden` element for `scrollWidth > clientWidth`.

### [P2] Restore badge in the pane header renders raw locale keys
- Area: `app/frontend/src/lib/SplitPane.svelte:365` (also :367 and :67)
- Platform: all
- Symptom: After a restart with a saved cmd/powershell/zsh-less (non bash/zsh/wsl) terminal, the pane header shows a badge reading `splitPane.restoredNewShellShort` and its tooltip reads `splitPane.restoredNewShell`. The tmux-badge tooltip of a rehydrated bash/SSH pane contains the literal `splitPane.restoredContinued`.
- Repro: 1. Mock `GetLoadedSessionData` to return `{ terminals: [{ type: 'cmd', name: 'Restored cmd', resumeId: 'sample-resume-2' }] }`. 2. Load the app. 3. Hover the dot badge next to the terminal-type chip. Observed (Playwright, EN): badge text `splitPane.restoredNewShellShort`, `title="splitPane.restoredNewShell"`.
- Cause: `SplitPane.svelte:361-368` calls `$t('splitPane.restoredNewShell')` / `$t('splitPane.restoredNewShellShort')` and `:67` calls `$t('splitPane.restoredContinued')`; none of the three keys exists in `en.js` or `de.js` (the `splitPane` block at `en.js:411-412` only has `restoredTranscript` / `closeRestored`). `i18n.js:55` falls back to the key string. Commit `4f25484` added the component strings without the locale entries.
- Fix sketch: Add the three keys to both locale files (short label for the badge, one-sentence tooltip, "continued" note for the tmux tooltip); consider a vitest that diffs used keys against the locales so this cannot regress.
- Evidence: `SplitPane.svelte:67,365,367`; `en.js:400-413`; `docs/audit/shots/i18n-copy-restore-badge-raw-key.png`

### [P2] Error banner messages are hardcoded English (one in German) regardless of locale
- Area: `app/frontend/src/lib/actions/templateActions.js:222` (representative; 30+ sites)
- Platform: all
- Symptom: In the German UI every red error banner at the top of the terminals page is English ("Please select a terminal first.", "Failed to apply template: …", "SSH connection failed: …"). The update flow instead shows German with an ASCII umlaut: "Update-Seite konnte nicht geoeffnet werden" even in the English UI.
- Repro: code reasoning, confidence high (all banner sources are literals, none pass through `$t`).
- Cause: `errorMessage.set(...)`/`$errorMessage = ...` with literals in `templateActions.js:222-344`, `sshActions.js:71,93,106`, `folderActions.js:31,41,55`, `terminalActions.js:228,431,501,531`, `aiSettingsActions.js:168`, `updateActions.js:25` (German, "geoeffnet"), `:36,41,51`, `App.svelte:172,177,204,234,252,325`, `AppMainContent.svelte:56`; rendered verbatim in `TerminalsPage.svelte:96`. The store holds a final string, so there is no late translation point.
- Fix sketch: Add an `errors.*` block to both locales and call `get(t)(key, vars)` at the set sites (the pattern already exists in `agentActions.js:325`); fix `updateActions.js:25` to English or a key with "geöffnet".
- Evidence: `grep -rn "errorMessage.set(" lib/actions`; `TerminalsPage.svelte:93-96`; `agentActions.js:325-329`

### [P2] Relative times and dates ignore the app locale
- Area: `app/frontend/src/lib/HistoryDashboard.svelte:127-131`
- Platform: all
- Symptom: History shows "just now", "5m ago", "3d ago" in the German UI; older rows are formatted as `04.10.26` even in the English UI (hardcoded `de-DE`). Recordings use the same English relative strings; Activity logs show "Unknown time" and `toLocaleString()` in the browser/OS locale, so an English-UI user on a German desktop sees `4.10.2026, 13:24:11` next to English labels (and vice versa).
- Repro: code reasoning, confidence high.
- Cause: `HistoryDashboard.svelte:127-131` (`'just now'`, `` `${diffMin}m ago` ``, `toLocaleDateString('de-DE', …)`); `RecordingPlayer.svelte:257-262` (`formatTimeAgo`, English literals); `ActivityLogViewer.svelte:46,52` (`'Unknown time'`, `toLocaleString()`); `TranscriptViewerModal.svelte:176` and `AgentTranscriptPanel.svelte:376` (`toLocaleTimeString([])` → browser locale, not `$locale`). Only `TranscriptViewerModal.formatRelative` (:165-177) uses `$t('transcriptViewer.justNow'|'minutesAgo'|'hoursAgo')`.
- Fix sketch: One shared `formatRelative(iso)`/`formatDateTime(iso)` in `i18n.js` that uses the existing `transcriptViewer.*` keys and `Intl.DateTimeFormat($locale)`; replace the three local copies.
- Evidence: `HistoryDashboard.svelte:120-135`; `RecordingPlayer.svelte:257-262`; `ActivityLogViewer.svelte:44-53`; `en.js:765-767`

### [P3] No plural forms: "1 commands", "1 Befehle", "1 Schritte"
- Area: `app/frontend/src/lib/i18n.js:41-57` (fill has no plural support)
- Platform: all
- Symptom: Every count label uses a single plural template, so `n = 1` renders "1 entries", "1 matches", "1 templates", "1 commands", "1 steps", "1 events", "1 sessions in this directory", "1 transcripts"; German likewise "1 Einträge", "1 Treffer" (ok), "1 Befehle", "1 Schritte", "1 Ereignisse", "1 Sitzungen". In the Playbooks pane every one-step playbook shows "1 steps".
- Repro: code reasoning, confidence high (WorkflowPlaybooksPane renders `stepsCount` for each playbook; the mock "Kubernetes Pod Triage" has one step).
- Cause: `en.js:513,527,544,586,662,669,680,693,774,845` and `de.js` same lines define only `'{n} X'`; call sites `HistoryDashboard.svelte:172`, `TemplateManager.svelte:529`, `WorkflowRunSummary.svelte:46`, `WorkflowBuilder.svelte:766,781`, `WorkflowPlaybooksPane.svelte:23,40`, `WorkflowFunctionCatalogPane.svelte:12`, `AgentTranscriptPanel.svelte:510`, `DotEnvViewerModal.svelte:192`.
- Fix sketch: Add `_one`/`_other` variants (or `Intl.PluralRules`) to `t()` and a `one:`/`other:` pair per count key; EN and DE both only need singular/plural.
- Evidence: `i18n.js:41-44`; `en.js:662`; `WorkflowPlaybooksPane.svelte:40`

### [P3] CSS `text-transform: lowercase` on translated chips lowercases German nouns ("freigabe nötig")
- Area: `app/frontend/src/lib/AgentTranscriptPanel.svelte:756`
- Platform: all
- Symptom: The status chip in the agent panel header shows "freigabe nötig" (DE) — the capitalised noun from `de.js:852` ("Freigabe nötig") is forced lowercase, which reads as a typo in German. "arbeitet"/"wartet" are unaffected.
- Repro: 1. DE locale, terminal with a Claude agent. 2. Emit `agent-state-1` with `state: 'permission'`. 3. Open the agent panel via the pane badge. Observed (Playwright): header text "freigabe nötig".
- Cause: `.agent-panel-status { text-transform: lowercase }` at `AgentTranscriptPanel.svelte:756` applied to `$t('agentPanel.permission')` at `:424`. The same rule exists on `.tmux-badge`/`.rc-badge` (`SplitPane.svelte:752,809`), currently only on backend tokens ("tmux", "rc").
- Fix sketch: Drop the lowercase transform and author the EN strings in the wanted case ("approval needed" already is); keep transforms off any element whose content comes from `$t`.
- Evidence: `AgentTranscriptPanel.svelte:424,756`; `de.js:852`

### [P3] Pane agent badge loses the space before the separator: "Claude· Freigabe" / "Claude· approval"
- Area: `app/frontend/src/lib/SplitPane.svelte:355`
- Platform: all
- Symptom: The badge in the pane header reads "Claude· Freigabe" (and "Claude· arbeitet", "Claude· approval" in EN) — no space between the label and the middle dot, while the sidebar row and the agent panel use "Claude · …".
- Repro: DE or EN, agent in `permission`/`working` state; observed in Playwright `textContent` = "Claude· Freigabe" and in the screenshot.
- Cause: `{agent.short || agent.label}{#if agent.status === 'permission'} · {$t('splitPane.agentPermission')}…` — Svelte trims the leading whitespace of the `{#if}` block's text, so only the trailing space survives.
- Fix sketch: Put the separator in the string (`{' · '}`), or move " · " into the locale value (`' · {label}'`) / a `<span class="sep">`.
- Evidence: `SplitPane.svelte:355`; `docs/audit/shots/i18n-copy-de-sidebar-permission.png`

### [P3] German terminology drifts across surfaces (Freigabe / Approval / Erlauben, Sitzung / Session, Templates / Vorlagen, AI / KI)
- Area: `app/frontend/src/lib/locales/de.js:305` (representative)
- Platform: all
- Symptom: The same concept is named differently depending on where the user looks: pane badge, sidebar and workflow dialogs say "Freigabe" (`de.js:25,45,403,677-707,852-856`), AI Settings / AI Hub say "Approval-Policy", "Approval für Low-Risk-Tools", "Immer Approval verlangen" (`de.js:141,240-241,261,305,330,342-346`), the answer button says "Erlauben" (`:853`). Agent sessions are "Sitzung(en)" in the workspace and panel (`:5-23,836-846`) but "Session" in the hook card and the rename field (`:128,136,805,808,812`). Templates are "Templates" everywhere except `templateManager.intro` ("Vorlagen", `:518`). "KI fragen" (`:216,298`) vs "AI Settings"/"AI-Konfiguration" (`:98,141,240,303`). "Nicht jetzt" (`:742`) vs "Jetzt nicht" (`:860`) for the same dismiss action.
- Repro: code reasoning, confidence high (string table).
- Cause: Blocks were translated at different times without a glossary.
- Fix sketch: Decide one term per concept (Freigabe, Sitzung, Template, AI or KI) and apply it in `de.js`; add a short glossary comment at the top of the file.
- Evidence: `de.js:25,45,141,216,305,518,742,808,853,860`

### [P3] ~25 German entries are still English (card titles, section headings, mode names)
- Area: `app/frontend/src/lib/locales/de.js:141`
- Platform: all
- Symptom: In the DE UI the Settings and AI-hub cards are titled "AI Settings", "Templates", "Notes", "Terminal Folders", "Command History", "GIF Export (agg)", "Updates", "Function Catalog", "AI Logs"; AI-settings sections "Prompt", "Execution", "AI Tool Intro / Pre-Prompt"; workflow modes "Assist / Approve / Auto"; file browser "Explorer"; player "Pause"; log viewer "Limit", "Prompt / Request"; history "Details"; builder "Playbooks", "Builder", "Prompt". The descriptions under them are German, so each card is bilingual.
- Repro: code reasoning, confidence high — 64 keys have byte-identical EN/DE values; after removing loanwords/proper nouns (Host, Port, Terminal, Workflow, Audit, Agents, Snippets, Bash, Shell …) about 25 remain untranslated.
- Cause: `de.js:141,142,145-150,177-180,239-241,296,303,305,310,335-337` etc. copied from `en.js` as placeholders.
- Fix sketch: Translate the remaining titles ("KI-Einstellungen", "Notizen", "Terminal-Ordner", "Befehlsverlauf", "Funktionskatalog", "KI-Protokolle", "Ausführung", "Vorschau") or consciously keep them as product terms and note it in the glossary.
- Evidence: script output "en === de identical strings (64)"; `de.js:141,240,241,303,305`

### [P3] English casing is inconsistent: Title Case in older blocks, sentence case in newer ones
- Area: `app/frontend/src/lib/locales/en.js:187`
- Platform: all
- Symptom: The same kind of button differs in case across pages: "Download & Install" (`en.js:187`) vs "Download & install" (`:93`), "Trust & Connect" (`:84`) vs "Create & unlock" (`:61`), "Approval Needed" (`:690`) vs "Needs attention" (`:10`), "Approve Workflow Step" / "Approve and Continue" (`:702,714`) vs "Install hook" / "Reload transcript" (`:858,861`). Headings: "SSH Hosts", "Terminal Folders", "Command History", "Keyboard Shortcuts", "Workflow Builder" vs "Agent workspace", "Workspace views", "Needs attention". 145 short strings are Title Case (script); the agentWorkspace / agentPanel / sidebar-agents blocks are consistently sentence case.
- Repro: code reasoning, confidence high.
- Cause: No casing rule; SSH / workflow / template / settings blocks predate the sentence-case blocks.
- Fix sketch: Pick sentence case (matches the newest UI and the German file) and normalise the ~145 strings; keep proper nouns.
- Evidence: `en.js:10,61,84,93,187,690,702,714`

### [P3] Hardcoded UI strings left in components
- Area: `app/frontend/src/lib/Sidebar.svelte:227`
- Platform: all
- Symptom: Untranslatable English in: sidebar terminal tooltip suffix `' - minimized'` (`Sidebar.svelte:227`, while the visible badge uses `$t('sidebar.minimized')`); template-manager success banners "Template updated/saved successfully!" (`TemplateManager.svelte:362,365`); workflow builder step labels "AI Step", "Workflow Step", "Playbook Tool", "Discovery: …" (`WorkflowBuilder.svelte:272-275`), approval result messages "Step approved. The workflow paused again…" / "Step approved and workflow continued." (`:503-504`), `title="Refresh discovery"` (`:828`); terminals toolbar button "AI" and status "AI: OpenAI/Ollama" (`TerminalsPage.svelte:64,88`), `aria-label="Resize notes panel"` (`:147`); `providerLabel` (`AIHubView.svelte:21`); "Unknown time" (`ActivityLogViewer.svelte:46`); desktop-notification title `${agent.label} · ${termName}` is fine, but `MarkdownNotes.svelte:86` has a dead `|| 'Please enter a filename'` fallback (`$t` never returns empty). Example placeholders in `AISettingsModal.svelte:133-196` ("Network, Kubernetes, AI", "AI Tool Run") are English-only but arguably data.
- Repro: code reasoning, confidence high (greps for text nodes, `title=`, `aria-label=`, success/error literals).
- Cause: Strings added after the i18n migration or in code paths (`successMessage`, step labels) that never went through `$t`.
- Fix sketch: Move the listed literals into `sidebar.*`, `templateManager.*`, `workflowBuilder.*`, `appTerminals.*`, `activityLog.*`; remove the dead fallback.
- Evidence: `Sidebar.svelte:227`; `TemplateManager.svelte:362-365`; `WorkflowBuilder.svelte:272-275,503-504,828`; `TerminalsPage.svelte:64,88,147`

### [P3] Numbers use a fixed "." decimal and English unit letters
- Area: `app/frontend/src/lib/HistoryDashboard.svelte:331`
- Platform: all
- Symptom: DE UI shows "98.5%" (expected "98,5 %"), "1.5 KB", "$0.12" (`toFixed`), token counts "12k / 1.2M", durations "3m 05s" / "2h 07m". The letters are compact and mono-styled, so the effect is cosmetic; the decimal separator is the visible one.
- Repro: code reasoning, confidence medium (depends on data being present; no overflow or wrong values).
- Cause: `HistoryDashboard.svelte:331` (`toFixed(1)` + `%`), `RecordingPlayer.svelte:266-267`, `TranscriptViewerModal.svelte:152-163` (`formatBytes`), `AgentTranscriptPanel.svelte:119-124,130-136,603` (`fmtTokens`, `fmtDuration`, `$…toFixed(2)`), `agentActions.js:413-421` (`elapsedSince`).
- Fix sketch: Route percentages/bytes/cost through `Intl.NumberFormat($locale)` in one helper; keep `s/m/h` and `k/M` as they are (both languages read them).
- Evidence: `HistoryDashboard.svelte:331`; `TranscriptViewerModal.svelte:162`; `AgentTranscriptPanel.svelte:121,603`

### [P3] Restore badge label is clipped at 120px — the German label does not fit
- Area: `app/frontend/src/lib/SplitPane.svelte:735`
- Platform: all
- Symptom: On hover the restore badge reveals its text inside `max-width: 120px; overflow: hidden; white-space: nowrap`. The German `splitPane.restoredTranscript` ("Wiederhergestelltes Transkript", 30 chars at 11px/600 ≈ 190px) and any German replacement for the missing `restoredNewShellShort` will be cut mid-word without an ellipsis. Live, the 31-char raw key already overflows by 65px (scan: `restore-text over=65 w=120`).
- Repro: 1. Restore a cmd/powershell terminal (see P2 above). 2. Hover the dot badge. Observed: text clipped at 120px, no ellipsis.
- Cause: `.restore-badge:hover .restore-text { max-width: 120px }` at `SplitPane.svelte:734-737`; `.restore-text` has no `text-overflow: ellipsis` (`:723-729`).
- Fix sketch: Raise `max-width` to ~200px or `max-width: 20ch`, add `text-overflow: ellipsis`, and keep the DE short label under ~16 characters ("Neue Shell" / "Nur Vorschau").
- Evidence: `SplitPane.svelte:723-737`; `de.js:411`; `docs/audit/shots/i18n-copy-restore-badge-raw-key.png`

### [P3] Dead locale keys
- Area: `app/frontend/src/lib/locales/en.js:830-845` (agentPanel block)
- Platform: all
- Symptom: No user-visible effect; maintenance noise and false sense of coverage.
- Repro: code reasoning, confidence medium (literal + template-prefix scan; a key built from a runtime value other than the scanned prefixes could still reach them).
- Cause: `agentPanel.viewFileTitle`, `agentPanel.viewScreenTitle`, `agentPanel.verified`, `settings.updatePanel.platform` are defined in both locales but never referenced. The remaining 35 "unreferenced" keys are reached via template keys (`agentWorkspace.statuses.${…}`, `settings.cards.tmuxMode.desc_${…}`, `settings.updatePanel.stage_${…}`, `aiPanel.titles.${…}`, `settings.languages.${…}`, `agentWorkspace.${live ? 'connected' : 'offline'}`) and are fine.
- Fix sketch: Delete the four keys or wire them in; keep the key-diff script as a test.
- Evidence: script output "en keys never referenced literally (39)"; `AgentWorkspaceView.svelte:164-269`; `SettingsView.svelte:80,110,139,317`

## Not found / verified OK

- EN/DE key parity: every key in `en.js` exists in `de.js` and vice versa (0 differences in both directions); `{n}`/`{count}`/`{name}` placeholders match 1:1 across all 876 lines (0 mismatches).
- All template-literal keys (`agentWorkspace.statuses.*`, `reasons.*`, `settings.cards.tmuxMode.desc_*`, `claudeHook.host_*`, `updatePanel.stage_*`, `aiPanel.titles.*`, `settings.languages.*`) resolve for every value the code can produce.
- No raw-key leak other than the three `splitPane.restored*` keys: the DE walk through terminals, agent panel, Settings and Agent workspace found no `block.key` pattern in body text, `title`, `aria-label` or `placeholder` (only the literal path `settings.json` in the hook card copy).
- German length vs. fixed-width places: sidebar headings, agent rows, "✓ Erlauben / ✕ Ablehnen", agent-panel tabs ("Snippets Dateien Befehle Prozesse Verlauf" at 439px), workspace chips ("Braucht Aufmerksamkeit 2 · Aufgaben 1 · Sitzungen 0"), Settings buttons ("Zurücksetzen" 88px, "Einrichten" 69px) all fit at 1280×800; every container uses `flex-wrap`. The overflow scan reported only the intentionally ellipsised `.sidebar-agent-text` (hook message) and the restore badge above. Screenshots: `docs/audit/shots/i18n-copy-de-sidebar-permission.png`, `i18n-copy-de-settings.png`.
- `title=`/`aria-label=`/`placeholder=` literals: all localized except the four listed in the hardcoded-strings finding (`TranscriptViewerModal.svelte:619 title="Ctrl+F"` is a shortcut, fine).
- ALL CAPS: no shouting in strings; the only caps are acronyms (ANSI, OSC, CMD, ADR-0011) and the deliberate "WARNING:" in `hostKey.mismatchWarning`. Uppercase sidebar/section headings come from CSS (`styles/sidebar.css:152`), which is fine for both languages.
- `sidebar.agentsShowArchived` ("show {n} archived" / "{n} archivierte anzeigen") reads correctly for n = 1 in both languages; `transcriptViewer.minutesAgo/hoursAgo` ("{n} min ago", "{n} h ago") are unit-based and need no plural.
- Language switch (`SettingsView.svelte:76-83`, `i18n.js:29-37`) is immediate, persisted in `localStorage['mimir-locale']`, and falls back to EN for unknown values.
- Desktop notification body is localized (`agentActions.js:325-329`); the hook's own one-line message (e.g. "Claude needs your permission to use Bash") is passed through as data, which is correct.
- Backend-sourced text (tmux errors in the tmux tooltip `SplitPane.svelte:69`, hook host status `SettingsView.svelte:140`, update errors `updateActions.js:36`) is English only — by design of the Go side, noted, not counted.
- Direction-sensitive CSS (note only): 18× `text-align: left`, 3× `text-align: right`, 11× `margin-left: auto`, no logical properties (`margin-inline-start`). Irrelevant while only EN/DE exist; worth switching to logical properties if an RTL locale is ever added.
