<script context="module">
  // Hook hint dismissed per terminal for this app session.
  const hookDismissed = new Set();
</script>

<script>
  // Side panel for the coding agent running in a terminal. It does not mirror
  // the conversation (the terminal next to it already shows that); it pulls
  // out what the terminal cannot hand over cleanly: exact snippets of the
  // last answer, the files the agent touched, the commands it ran. Data comes
  // from the agent's own session file; the tmux capture is the fallback.
  import { onDestroy } from 'svelte';
  import { marked } from 'marked';
  import { ClipboardSetText } from '../../wailsjs/runtime';
  import { t } from './i18n.js';
  import { sanitizeHtml } from './util.js';
  import { extractSnippets, firstProse, groupTurns, splitMarkdown } from './agents/markdownBlocks.js';
  import { agentAnnotations, agentStates } from './stores/agentStore.js';
  import { activeTerminalId, terminalMap } from './stores/terminalStore.js';
  import { notesPanelOpen } from './stores/uiStore.js';
  import { answerAgentPermission, closeAgentPanel, elapsedSince, loadAgentGitStatus, loadAgentPaneText, loadAgentProcesses, loadAgentSessions, loadAgentTranscript, loadClaudeHookStatus, projectFolderName, projectKey, selectAgentSession, sessionKey, setClaudeHookInstalled, setProjectName, setSessionAnnotation } from './actions/agentActions.js';

  export let terminalId;

  const REFRESH_WORKING_MS = 6000;
  const MESSAGE_LIMIT = 24;
  const ALL_TABS = ['snippets', 'tasks', 'files', 'commands', 'processes', 'history', 'screen'];
  const PROCESS_REFRESH_MS = 5000;

  let transcript = null;
  let view = 'snippets';
  let pane = null;
  let paneFull = false;
  let paneLoading = false;
  let git = null;
  let gitLoading = false;
  let loading = false;
  let error = '';
  let feedback = '';
  let feedbackTimer = null;
  let refreshTimer = null;
  let refreshSoonTimer = null;
  let loadedFor = null;
  let showOlderSnippets = false;
  let summaryExpanded = false;
  let sessions = null;       // { sessions: [...], selected: '' } once loaded
  let sessionsOpen = false;
  // Claude Code approval hook on the agent's host: null until loaded.
  let hook = null;
  let hookBusy = false;
  let hookHintFor = null;
  // Process tree (what runs under the agent right now); refreshed while the
  // tab is open, faster while the agent works.
  // Session name / note / archived and the project name: the only state
  // Mimir keeps about an agent across restarts.
  $: note = $agentAnnotations.sessions[sessionKey(agent)] || null;
  $: projectName = $agentAnnotations.projects[projectKey(agent)]?.name || projectFolderName(agent);
  let editingName = false;
  let nameDraft = '';
  let editingProject = false;
  let projectDraft = '';
  let noteOpen = false;
  let noteDraft = '';
  let noteTimer = null;
  $: if (!noteOpen) noteDraft = note?.note || '';
  async function saveName() {
    editingName = false;
    try { await setSessionAnnotation(terminalId, { name: nameDraft.trim(), note: note?.note || '', archived: !!note?.archived }); } catch (e) { showFeedback(String(e?.message || e)); }
  }
  async function saveProject() {
    editingProject = false;
    try { await setProjectName(terminalId, projectDraft.trim()); } catch (e) { showFeedback(String(e?.message || e)); }
  }
  function noteChanged() {
    if (noteTimer) clearTimeout(noteTimer);
    noteTimer = setTimeout(async () => {
      try { await setSessionAnnotation(terminalId, { name: note?.name || '', note: noteDraft.trim(), archived: !!note?.archived }); } catch (e) { showFeedback(String(e?.message || e)); }
    }, 600);
  }
  async function toggleArchived() {
    try { await setSessionAnnotation(terminalId, { name: note?.name || '', note: note?.note || '', archived: !note?.archived }); } catch (e) { showFeedback(String(e?.message || e)); }
  }
  let procs = null;
  let procsLoading = false;
  let procsTimer = null;
  let now = Date.now();
  let clock = null;
  $: if (agent?.status === 'working' && agent?.activity && !clock) clock = setInterval(() => { now = Date.now(); }, 1000);
  $: if (!(agent?.status === 'working' && agent?.activity) && clock) { clearInterval(clock); clock = null; }
  $: activityLine = agent?.status === 'working' && agent?.activity ? `${agent.activity}${elapsedSince(agent.activityAt, now) ? ' · ' + elapsedSince(agent.activityAt, now) : ''}` : '';
  $: if (view === 'processes') scheduleProcs(); else stopProcs();

  async function refreshProcs() {
    if (procsLoading) return;
    procsLoading = true;
    try { procs = await loadAgentProcesses(terminalId); } catch (e) { procs = { processes: [], reason: String(e?.message || e) }; } finally { procsLoading = false; }
  }
  function scheduleProcs() {
    if (procsTimer) return;
    refreshProcs();
    procsTimer = setInterval(refreshProcs, PROCESS_REFRESH_MS);
  }
  function stopProcs() {
    if (procsTimer) { clearInterval(procsTimer); procsTimer = null; }
  }
  function procLabel(args) {
    const s = String(args || '');
    return s.length > 160 ? s.slice(0, 160) + '…' : s;
  }

  $: agent = $agentStates[terminalId] || null;
  $: term = $terminalMap.get(terminalId) || null;
  // The screen view needs tmux (capture-pane); PowerShell/cmd and tmux mode
  // "off" have none.
  $: TABS = ALL_TABS.filter((t) => (t !== 'screen' || !(agent && agent.tmux === false)) && (t !== 'tasks' || (transcript?.tasks || []).length > 0));
  $: openTasks = (transcript?.tasks || []).filter((t) => t.status !== 'completed' && t.status !== 'cancelled').length;
  $: if (view === 'tasks' && !TABS.includes('tasks')) view = 'snippets';
  $: if (view === 'screen' && !TABS.includes('screen')) view = 'snippets';
  // "Insert" targets the pane the user last clicked (the active terminal),
  // not the agent's own pane: snippets are meant for the other terminals.
  // Falls back to the agent's pane when nothing else is active.
  $: insertTarget = ($activeTerminalId != null && $terminalMap.get($activeTerminalId)) || term;
  $: insertTitle = insertTarget ? $t('agentPanel.insertInto', { name: insertTarget.name }) : '';
  $: if (terminalId !== loadedFor) { loadedFor = terminalId; resetFor(); refresh(); }
  $: if (agent?.kind === 'claude' && hookHintFor !== terminalId) { hookHintFor = terminalId; hook = null; loadHook(); }
  $: hookHintVisible = agent?.kind === 'claude' && hook && !hook.installed && !hook.error && !hookDismissed.has(terminalId);

  async function loadHook() {
    try { hook = await loadClaudeHookStatus(terminalId); } catch { hook = null; }
  }
  async function installHook() {
    hookBusy = true;
    try {
      hook = await setClaudeHookInstalled(terminalId, true);
      showFeedback($t('agentPanel.hookInstalled'));
    } catch (e) {
      showFeedback(String(e?.message || e));
    } finally {
      hookBusy = false;
    }
  }
  $: if (agent?.status === 'idle' && agent?.lastChange) refreshSoon();
  $: scheduleAutoRefresh(agent?.status);

  $: assistantMessages = (transcript?.messages || []).filter((m) => m.role === 'assistant');
  // A reply can be spread over several assistant records (text, tool call,
  // text, ...); the unit the user cares about is the whole turn since their
  // last prompt.
  $: turns = groupTurns(transcript?.messages || []);
  $: lastTurn = turns.length ? turns[turns.length - 1] : null;
  $: lastAnswer = lastTurn ? { text: lastTurn.answers.map((a) => a.text).join('\n\n'), timestamp: lastTurn.answers[lastTurn.answers.length - 1].timestamp } : null;
  $: lastUserPrompt = lastTurn?.prompt || null;
  $: summary = lastAnswer ? firstProse(lastAnswer.text, summaryExpanded ? 4000 : 320) : '';
  $: snippetGroups = buildSnippetGroups(turns, showOlderSnippets);
  $: commandsNewestFirst = [...(transcript?.commands || [])].reverse();
  $: failedCommands = (transcript?.commands || []).filter((c) => c.failed).length;
  $: changedFiles = (transcript?.files || []).filter((f) => f.ops.some((op) => op !== 'read'));

  function resetFor() {
    transcript = null; pane = null; paneFull = false; git = null; view = 'snippets';
    error = ''; showOlderSnippets = false; summaryExpanded = false; sessions = null; sessionsOpen = false;
  }

  async function toggleSessions() {
    sessionsOpen = !sessionsOpen;
    if (sessionsOpen && !sessions) {
      try {
        sessions = await loadAgentSessions(terminalId);
      } catch (e) {
        error = String(e?.message || e);
        sessionsOpen = false;
      }
    }
  }

  async function chooseSession(event) {
    const file = event.target.value;
    try {
      await selectAgentSession(terminalId, file);
      if (sessions) sessions = { ...sessions, selected: file };
      sessionsOpen = false;
      transcript = null;
      await refresh();
      flash($t('agentPanel.sessionPinned'));
    } catch (e) {
      error = String(e?.message || e);
    }
  }

  function sessionLabel(s) {
    const when = shortTime(s.modified);
    const title = s.title || s.file.split(/[\\/#]/).pop();
    return `${when ? when + ' · ' : ''}${title}`;
  }

  function buildSnippetGroups(allTurns, includeOlder) {
    const source = includeOlder ? allTurns.slice(-4) : allTurns.slice(-1);
    return source
      .map((turn, i) => {
        const last = turn.answers[turn.answers.length - 1];
        const seen = new Set();
        const snippets = [];
        for (const answer of turn.answers) {
          for (const snippet of extractSnippets(answer.text)) {
            const key = `${snippet.type}:${snippet.code}`;
            if (seen.has(key)) continue;
            seen.add(key);
            snippets.push(snippet);
          }
        }
        return { key: `${last.timestamp || ''}-${i}`, timestamp: last.timestamp, prompt: turn.prompt?.text || '', snippets };
      })
      .filter((g) => g.snippets.length)
      .reverse();
  }

  function scheduleAutoRefresh(status) {
    if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null; }
    if (status === 'working') refreshTimer = setInterval(refresh, REFRESH_WORKING_MS);
  }

  function refreshSoon() {
    if (refreshSoonTimer) clearTimeout(refreshSoonTimer);
    refreshSoonTimer = setTimeout(refresh, 800);
  }

  async function refresh() {
    if (terminalId == null) return;
    if (view === 'screen') return refreshPane();
    if (loading) return;
    loading = true;
    try {
      transcript = await loadAgentTranscript(terminalId, MESSAGE_LIMIT);
      error = '';
      if (view === 'files' && git) refreshGit();
    } catch (e) {
      error = String(e?.message || e);
    } finally {
      loading = false;
    }
  }

  async function refreshPane(full = paneFull) {
    if (paneLoading || terminalId == null) return;
    paneLoading = true;
    paneFull = full;
    try {
      pane = await loadAgentPaneText(terminalId, { full });
      error = '';
    } catch (e) {
      error = String(e?.message || e);
    } finally {
      paneLoading = false;
    }
  }

  async function refreshGit() {
    if (gitLoading || terminalId == null) return;
    gitLoading = true;
    try {
      git = await loadAgentGitStatus(terminalId);
      error = '';
    } catch (e) {
      error = String(e?.message || e);
    } finally {
      gitLoading = false;
    }
  }

  function showView(next) {
    view = next;
    if (next === 'screen' && !pane) refreshPane();
    else if (next !== 'screen' && !transcript) refresh();
    if (next === 'files' && !git) refreshGit();
  }

  function flash(message) {
    feedback = message;
    if (feedbackTimer) clearTimeout(feedbackTimer);
    feedbackTimer = setTimeout(() => { feedback = ''; }, 1800);
  }

  async function copyText(text) {
    try {
      await ClipboardSetText(text);
      flash($t('agentPanel.copied'));
    } catch (e) {
      error = `Copy failed: ${e?.message || e}`;
    }
  }

  function insertIntoTerminal(code) {
    const target = insertTarget;
    const xterm = target?.terminal;
    if (!xterm || typeof xterm.paste !== 'function') return;
    // paste() applies bracketed paste, so multi-line snippets are not executed
    // line by line; the user still confirms with Enter.
    xterm.paste(code.replace(/\n+$/, ''));
    xterm.focus?.();
    flash($t('agentPanel.inserted', { name: target.name }));
  }

  async function saveToNotes(code, lang = '') {
    const stamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
    const filename = `agent-${agent?.kind || 'snippet'}-${stamp}.md`;
    const body = '```' + lang + '\n' + code + '\n```\n';
    try {
      await window['go']['main']['App']['SaveNote'](filename, body);
      notesPanelOpen.set(true);
      flash($t('agentPanel.savedToNotes', { filename }));
    } catch (e) {
      error = `Save failed: ${e?.message || e}`;
    }
  }

  async function openFileInNotes(path) {
    try {
      const app = window['go']['main']['App'];
      if (agent?.source === 'ssh') await app['ImportNoteFromRemote'](terminalId, path);
      else await app['ImportNoteFromLocal'](path);
      notesPanelOpen.set(true);
      flash($t('agentPanel.openedInNotes'));
    } catch (e) {
      error = `Open failed: ${e?.message || e}`;
    }
  }

  function renderProse(text) {
    return sanitizeHtml(marked(text || ''));
  }

  function shortTime(ts) {
    if (!ts) return '';
    const d = new Date(ts);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function shortPath(p) {
    if (!p) return '';
    const parts = p.split(/[\\/]/).filter(Boolean);
    return parts.length > 2 ? '…/' + parts.slice(-2).join('/') : p;
  }

  function relPath(p) {
    const cwd = transcript?.cwd || agent?.cwd || '';
    if (cwd && p.startsWith(cwd + '/')) return p.slice(cwd.length + 1);
    return p;
  }

  function tabLabel(tab) {
    switch (tab) {
      case 'snippets': return $t('agentPanel.tabSnippets');
      case 'tasks': return $t('agentPanel.tabTasks') + (openTasks ? ` (${openTasks})` : '');
      case 'files': return $t('agentPanel.tabFiles') + (changedFiles.length ? ` (${changedFiles.length})` : '');
      case 'commands': return $t('agentPanel.tabCommands') + (failedCommands ? ` (${failedCommands}✗)` : '');
      case 'processes': return $t('agentPanel.tabProcesses') + (procs?.processes?.length > 1 ? ` (${procs.processes.length})` : '');
      case 'history': return $t('agentPanel.tabHistory');
      default: return $t('agentPanel.viewScreen');
    }
  }

  onDestroy(() => {
    if (noteTimer) clearTimeout(noteTimer);
    stopProcs();
    if (clock) clearInterval(clock);
    if (refreshTimer) clearInterval(refreshTimer);
    if (refreshSoonTimer) clearTimeout(refreshSoonTimer);
    if (feedbackTimer) clearTimeout(feedbackTimer);
  });
</script>

<div class="agent-panel-inner">
  <div class="agent-panel-header">
    <div class="agent-panel-title">
      {#if editingName}
        <!-- svelte-ignore a11y_autofocus -->
        <input class="agent-inline-input" type="text" bind:value={nameDraft} maxlength="120" placeholder={agent?.label || ''} autofocus on:keydown={(e) => { if (e.key === 'Enter') saveName(); if (e.key === 'Escape') editingName = false; }} on:blur={saveName} />
      {:else}
        <button type="button" class="agent-panel-label agent-panel-label-btn" title={$t('agentPanel.renameSession')} disabled={!sessionKey(agent)} on:click={() => { nameDraft = note?.name || ''; editingName = true; }}>{note?.name ? note.name + ' · ' : ''}{agent?.label || $t('agentPanel.title')}{#if note?.archived} · {$t('agentPanel.archived')}{/if}</button>
      {/if}
      {#if agent?.status && agent.status !== 'unknown'}
        <span class="agent-panel-status agent-status-{agent.status}">{agent.status === 'permission' ? $t('agentPanel.permission') : agent.status === 'working' ? $t('agentPanel.working') : $t('agentPanel.idle')}</span>
      {/if}
      <span class="agent-panel-sub" title={transcript?.cwd || agent?.cwd || ''}>{term?.name || ''}{agent?.cwd ? ' · ' + shortPath(agent.cwd) : ''}</span>
      {#if activityLine}<span class="agent-panel-activity" title={agent.activity}>{activityLine}</span>{/if}
      <span class="agent-panel-project">
        {#if editingProject}
          <!-- svelte-ignore a11y_autofocus -->
          <input class="agent-inline-input" type="text" bind:value={projectDraft} maxlength="120" autofocus on:keydown={(e) => { if (e.key === 'Enter') saveProject(); if (e.key === 'Escape') editingProject = false; }} on:blur={saveProject} />
        {:else}
          <button type="button" class="agent-link" title={$t('agentPanel.renameProject')} disabled={!projectKey(agent)} on:click={() => { projectDraft = $agentAnnotations.projects[projectKey(agent)]?.name || ''; editingProject = true; }}>{$t('agentPanel.project')}: {projectName || '—'}</button>
        {/if}
        {#if agent?.project?.branch}<span class="agent-row-dim" title={agent.project.worktree ? $t('agentPanel.worktree') : ''}>{agent.project.worktree ? '⎇ ' : ''}{agent.project.branch}</span>{/if}
        <button type="button" class="agent-link" disabled={!sessionKey(agent)} on:click={() => { noteOpen = !noteOpen; }}>{$t('agentPanel.note')}{note?.note ? ' ●' : ''}</button>
        <button type="button" class="agent-link" disabled={!sessionKey(agent)} on:click={toggleArchived}>{note?.archived ? $t('agentPanel.unarchive') : $t('agentPanel.archive')}</button>
      </span>
    </div>
    <div class="agent-panel-actions">
      <button type="button" class="agent-btn" on:click={() => (view === 'screen' ? refreshPane() : refresh())} disabled={loading || paneLoading} title={$t('agentPanel.refresh')}>{loading || paneLoading ? '…' : '↻'}</button>
      <button type="button" class="agent-btn" on:click={closeAgentPanel} title={$t('agentPanel.close')}>&#x2715;</button>
    </div>
  </div>

  {#if noteOpen}
    <textarea class="agent-note" rows="3" maxlength="4000" placeholder={$t('agentPanel.notePlaceholder')} bind:value={noteDraft} on:input={noteChanged}></textarea>
  {/if}
  {#if agent?.status === 'permission' && agent.lastText}
    <p class="agent-panel-hint agent-panel-permission">
      <span>{agent.lastText}</span>
      {#if agent.prompt === 'permission_prompt'}
        <button type="button" class="agent-btn agent-btn-allow" disabled={agent.answering} on:click={() => answerAgentPermission(terminalId, true)}>✓ {$t('agentPanel.allow')}</button>
        <button type="button" class="agent-btn agent-btn-deny" disabled={agent.answering} on:click={() => answerAgentPermission(terminalId, false)}>✕ {$t('agentPanel.deny')}</button>
      {/if}
    </p>
  {/if}
  {#if hookHintVisible}
    <p class="agent-panel-hint agent-panel-hook">
      <span>{$t('agentPanel.hookHint')}</span>
      <button type="button" class="agent-btn" on:click={installHook} disabled={hookBusy}>{hookBusy ? '…' : $t('agentPanel.hookInstall')}</button>
      <button type="button" class="agent-btn" on:click={() => { hookDismissed.add(terminalId); hookHintVisible = false; }} title={$t('agentPanel.hookDismiss')}>&#x2715;</button>
    </p>
  {/if}

  <div class="agent-tabs" role="tablist">
    {#each TABS as tab (tab)}
      <button type="button" class="agent-tab {view === tab ? 'agent-tab-active' : ''}" role="tab" aria-selected={view === tab} on:click={() => showView(tab)}>{tabLabel(tab)}</button>
    {/each}
  </div>

  {#if feedback}
    <div class="agent-panel-feedback">{feedback}</div>
  {/if}
  {#if error}
    <div class="agent-panel-error">{error}</div>
  {/if}

  <div class="agent-panel-body">
    {#if view === 'screen'}
      <p class="agent-panel-hint">{$t('agentPanel.screenHint')}</p>
      {#if pane}
        <div class="agent-code">
          <div class="agent-code-bar">
            <span class="agent-code-lang">tmux · {pane.lines} {$t('agentPanel.lines')} · {pane.width} {$t('agentPanel.cols')}</span>
            {#if pane.hiddenLines > 0}
              <button type="button" class="agent-link" on:click={() => refreshPane(true)} title={$t('agentPanel.showAllTitle', { n: pane.hiddenLines })}>{$t('agentPanel.showAll', { n: pane.hiddenLines })}</button>
            {:else if paneFull}
              <button type="button" class="agent-link" on:click={() => refreshPane(false)}>{$t('agentPanel.showSession')}</button>
            {/if}
            <button type="button" class="agent-link" on:click={() => copyText(pane.text)}>{$t('agentPanel.copy')}</button>
          </div>
          <pre><code>{pane.text}</code></pre>
        </div>
      {:else if paneLoading}
        <p class="agent-panel-hint">{$t('agentPanel.loading')}</p>
      {/if}

    {:else if !transcript && loading}
      <p class="agent-panel-hint">{$t('agentPanel.loading')}</p>
    {:else if !transcript}
      <p class="agent-panel-hint">{$t('agentPanel.empty')}</p>
    {:else}
      {#if transcript.source === 'tmux'}
        <p class="agent-panel-hint agent-panel-warn">{$t('agentPanel.fallbackHint')}</p>
      {:else if !transcript.verified || transcript.candidates > 1}
        <p class="agent-panel-hint {transcript.verified ? '' : 'agent-panel-warn'}" title={transcript.sessionFile}>
          {#if transcript.verified}
            {$t('agentPanel.candidates', { n: transcript.candidates })}
          {:else if agent && agent.tmux === false}
            {$t('agentPanel.unverifiedNoTmux')}{#if transcript.candidates > 1} · {$t('agentPanel.candidates', { n: transcript.candidates })}{/if}
          {:else}
            {$t('agentPanel.unverified')}{#if transcript.candidates > 1} · {$t('agentPanel.candidates', { n: transcript.candidates })}{/if}
          {/if}
          {#if transcript.candidates > 1 || sessions}
            <button type="button" class="agent-link" on:click={toggleSessions}>{$t('agentPanel.chooseSession')}</button>
          {/if}
        </p>
        {#if sessionsOpen && sessions}
          <label class="agent-session-pick">
            <span>{$t('agentPanel.sessionPickLabel')}</span>
            <select value={sessions.selected || ''} on:change={chooseSession}>
              <option value="">{$t('agentPanel.sessionAuto')}</option>
              {#each sessions.sessions as s (s.file)}
                <option value={s.file}>{sessionLabel(s)}</option>
              {/each}
            </select>
          </label>
        {/if}
      {/if}

      {#if view === 'snippets'}
        {#if lastAnswer}
          <div class="agent-summary">
            <div class="agent-msg-meta">
              <span>{agent?.status === 'idle' ? $t('agentPanel.lastAnswerIdle') : $t('agentPanel.lastAnswer')}</span>
              <span>{shortTime(lastAnswer.timestamp)}</span>
              <button type="button" class="agent-link" on:click={() => copyText(lastAnswer.text)}>{$t('agentPanel.copyMessage')}</button>
            </div>
            {#if lastUserPrompt}
              <div class="agent-summary-prompt" title={lastUserPrompt.text}>❯ {lastUserPrompt.text.length > 140 ? lastUserPrompt.text.slice(0, 140) + '…' : lastUserPrompt.text}</div>
            {/if}
            <div class="agent-summary-text">{summary}</div>
            {#if lastAnswer.text.length > 320}
              <button type="button" class="agent-link" on:click={() => (summaryExpanded = !summaryExpanded)}>{summaryExpanded ? $t('agentPanel.less') : $t('agentPanel.more')}</button>
            {/if}
          </div>
        {/if}
        {#if snippetGroups.length === 0}
          <p class="agent-panel-hint">{$t('agentPanel.noSnippets')}</p>
        {/if}
        {#each snippetGroups as group (group.key)}
          {#if showOlderSnippets}
            <div class="agent-group-label" title={group.prompt}>{shortTime(group.timestamp)}{group.prompt ? ' · ❯ ' + (group.prompt.length > 60 ? group.prompt.slice(0, 60) + '…' : group.prompt) : ''}</div>
          {/if}
          {#each group.snippets as snippet, i (i)}
            {#if snippet.type === 'code'}
              <div class="agent-code">
                <div class="agent-code-bar">
                  <span class="agent-code-lang">{snippet.lang || 'code'}</span>
                  <button type="button" class="agent-link" on:click={() => copyText(snippet.code)}>{$t('agentPanel.copy')}</button>
                  <button type="button" class="agent-link" on:click={() => insertIntoTerminal(snippet.code)} disabled={!insertTarget} title={insertTitle}>{$t('agentPanel.insert')}{#if insertTarget && insertTarget.id !== terminalId} → {insertTarget.name}{/if}</button>
                  <button type="button" class="agent-link" on:click={() => saveToNotes(snippet.code, snippet.lang)}>{$t('agentPanel.toNotes')}</button>
                </div>
                <pre><code>{snippet.code}</code></pre>
              </div>
            {:else}
              <div class="agent-inline">
                <code>{snippet.code}</code>
                <button type="button" class="agent-link" on:click={() => copyText(snippet.code)}>{$t('agentPanel.copy')}</button>
                <button type="button" class="agent-link" on:click={() => insertIntoTerminal(snippet.code)} disabled={!insertTarget} title={insertTitle}>{$t('agentPanel.insert')}{#if insertTarget && insertTarget.id !== terminalId} → {insertTarget.name}{/if}</button>
              </div>
            {/if}
          {/each}
        {/each}
        {#if turns.length > 1}
          <button type="button" class="agent-link agent-more" on:click={() => (showOlderSnippets = !showOlderSnippets)}>{showOlderSnippets ? $t('agentPanel.olderHide') : $t('agentPanel.olderShow')}</button>
        {/if}

      {:else if view === 'tasks'}
        <p class="agent-panel-hint">{$t('agentPanel.tasksHint')}</p>
        <ul class="agent-tasks">
          {#each transcript.tasks as task, i (i)}
            <li class="agent-task agent-task-{task.status}">
              <span class="agent-task-mark">{task.status === 'completed' ? '✔' : task.status === 'in_progress' ? '●' : task.status === 'cancelled' ? '✕' : '○'}</span>
              <span class="agent-task-text">{task.content}</span>
              {#if task.priority === 'high'}<span class="agent-task-prio">!</span>{/if}
            </li>
          {/each}
        </ul>
      {:else if view === 'files'}
        <div class="agent-section-head">
          <span>{$t('agentPanel.gitTitle')}</span>
          <button type="button" class="agent-link" on:click={refreshGit} disabled={gitLoading}>{gitLoading ? '…' : $t('agentPanel.gitReload')}</button>
        </div>
        {#if git && !git.isRepo}
          <p class="agent-panel-hint">{$t('agentPanel.gitNotRepo')}</p>
        {:else if git}
          {#if git.status}
            <pre class="agent-pre">{git.status}</pre>
          {:else}
            <p class="agent-panel-hint">{$t('agentPanel.gitClean')}</p>
          {/if}
          {#if git.diffStat}
            <pre class="agent-pre agent-pre-dim">{git.diffStat}</pre>
          {/if}
        {/if}
        <div class="agent-section-head"><span>{$t('agentPanel.filesTitle', { n: transcript.files.length })}</span></div>
        {#if transcript.files.length === 0}
          <p class="agent-panel-hint">{$t('agentPanel.noFiles')}</p>
        {/if}
        {#each transcript.files as file (file.path)}
          <div class="agent-row" title={file.path}>
            <span class="agent-ops">
              {#each file.ops as op (op)}<span class="agent-op agent-op-{op}">{op}</span>{/each}
            </span>
            <code class="agent-row-main">{relPath(file.path)}</code>
            <span class="agent-row-dim">{file.count > 1 ? '×' + file.count : ''} {shortTime(file.lastAt)}</span>
            <button type="button" class="agent-link" on:click={() => copyText(file.path)}>{$t('agentPanel.copy')}</button>
            {#if agent?.source !== 'wsl'}
              <button type="button" class="agent-link" on:click={() => openFileInNotes(file.path)}>{$t('agentPanel.toNotes')}</button>
            {/if}
          </div>
        {/each}

      {:else if view === 'processes'}
        <div class="agent-section-head">
          <span>{$t('agentPanel.processesTitle')}</span>
          <button type="button" class="agent-link" on:click={refreshProcs} disabled={procsLoading}>{procsLoading ? '…' : $t('agentPanel.gitReload')}</button>
        </div>
        {#if !procs}
          <p class="agent-panel-hint">{$t('agentPanel.loading')}</p>
        {:else if procs.reason}
          <p class="agent-panel-hint">{procs.reason}</p>
        {:else}
          {#each procs.processes as p (p.pid)}
            <div class="agent-row agent-proc-row" style="padding-left: {8 + p.depth * 14}px" title={p.args}>
              <span class="agent-row-dim agent-proc-pid">{p.pid}</span>
              <code class="agent-row-main">{procLabel(p.args)}</code>
              <span class="agent-row-dim">{p.elapsed || ''}{p.cpu ? ' · ' + p.cpu + (p.cpu.endsWith('s') ? '' : '%') : ''}</span>
            </div>
          {/each}
          <p class="agent-panel-hint">{$t('agentPanel.processesHint')}</p>
        {/if}
      {:else if view === 'commands'}
        {#if commandsNewestFirst.length === 0}
          <p class="agent-panel-hint">{$t('agentPanel.noCommands')}</p>
        {/if}
        {#each commandsNewestFirst as cmd, i (i)}
          <div class="agent-cmd {cmd.failed ? 'agent-cmd-failed' : ''}">
            <div class="agent-msg-meta">
              <span class="agent-exit {cmd.hasExit ? (cmd.failed ? 'agent-exit-fail' : 'agent-exit-ok') : ''}" title={cmd.hasExit ? `exit ${cmd.exitCode}` : ''}>{cmd.hasExit ? (cmd.failed ? '✗ ' + cmd.exitCode : '✓') : '…'}</span>
              <span>{shortTime(cmd.at)}</span>
              {#if cmd.description}<span class="agent-row-dim">{cmd.description}</span>{/if}
              <button type="button" class="agent-link" on:click={() => copyText(cmd.command)}>{$t('agentPanel.copy')}</button>
              <button type="button" class="agent-link" on:click={() => insertIntoTerminal(cmd.command)} disabled={!insertTarget} title={insertTitle}>{$t('agentPanel.insert')}{#if insertTarget && insertTarget.id !== terminalId} → {insertTarget.name}{/if}</button>
            </div>
            <pre><code>{cmd.command}</code></pre>
          </div>
        {/each}

      {:else}
        {#if transcript.truncated}
          <p class="agent-panel-hint">{$t('agentPanel.truncated', { n: transcript.messages.length })}</p>
        {/if}
        {#each transcript.messages as message, index (index)}
          <div class="agent-msg agent-msg-{message.role}">
            <div class="agent-msg-meta">
              <span>{message.role === 'user' ? $t('agentPanel.you') : (agent?.label || transcript.label)}</span>
              <span>{shortTime(message.timestamp)}</span>
              {#if message.role === 'assistant'}
                <button type="button" class="agent-link" on:click={() => copyText(message.text)}>{$t('agentPanel.copyMessage')}</button>
              {/if}
            </div>
            {#if message.role === 'user'}
              <div class="agent-msg-user">{message.text}</div>
            {:else}
              {#each splitMarkdown(message.text) as segment, i (i)}
                {#if segment.type === 'text'}
                  <div class="agent-prose">{@html renderProse(segment.text)}</div>
                {:else}
                  <div class="agent-code">
                    <div class="agent-code-bar">
                      <span class="agent-code-lang">{segment.lang || 'code'}</span>
                      <button type="button" class="agent-link" on:click={() => copyText(segment.code)}>{$t('agentPanel.copy')}</button>
                      <button type="button" class="agent-link" on:click={() => insertIntoTerminal(segment.code)} disabled={!insertTarget} title={insertTitle}>{$t('agentPanel.insert')}{#if insertTarget && insertTarget.id !== terminalId} → {insertTarget.name}{/if}</button>
                      <button type="button" class="agent-link" on:click={() => saveToNotes(segment.code, segment.lang)}>{$t('agentPanel.toNotes')}</button>
                    </div>
                    <pre><code>{segment.code}</code></pre>
                  </div>
                {/if}
              {/each}
            {/if}
          </div>
        {/each}
      {/if}
    {/if}
  </div>
  <div class="agent-panel-footer" title={transcript?.sessionFile || ''}>
    {#if transcript?.sessionFile}{transcript.verified ? '✓ ' : ''}{$t('agentPanel.source')}: {shortPath(transcript.sessionFile)}{/if}
  </div>
</div>

<style>
  .agent-panel-inner { display: flex; flex-direction: column; height: 100%; min-height: 0; font-size: 12px; }
  .agent-panel-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; padding: 8px 10px 4px; }
  .agent-panel-title { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; min-width: 0; }
  .agent-panel-label { font-weight: 700; }
  .agent-panel-label-btn { background: transparent; border: 0; color: inherit; padding: 0; cursor: text; font: inherit; font-weight: 700; }
  .agent-panel-label-btn:disabled { cursor: default; }
  .agent-inline-input { background: var(--bg-elevated, rgba(255,255,255,0.06)); border: 1px solid var(--border-subtle); color: inherit; border-radius: 4px; padding: 1px 6px; font: inherit; min-width: 12ch; }
  .agent-panel-project { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; width: 100%; font-size: 11px; }
  .agent-note { margin: 0 10px 6px; width: calc(100% - 20px); resize: vertical; background: var(--bg-elevated, rgba(255,255,255,0.04)); border: 1px solid var(--border-subtle); color: inherit; border-radius: 4px; padding: 4px 6px; font: inherit; font-size: 12px; }
  .agent-panel-sub { color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
  .agent-panel-status { border-radius: 999px; padding: 1px 7px; font-size: 10px; font-weight: 700; text-transform: lowercase; border: 1px solid transparent; }
  .agent-status-working { background: rgba(227, 179, 65, 0.14); color: #e3b341; border-color: rgba(227, 179, 65, 0.32); }
  .agent-status-idle { background: rgba(126, 231, 135, 0.14); color: #7ee787; border-color: rgba(126, 231, 135, 0.28); }
  .agent-status-permission { background: rgba(255, 123, 114, 0.16); color: #ff7b72; border-color: rgba(255, 123, 114, 0.5); }
  .agent-panel-activity { font-family: var(--font-mono, monospace); font-size: 11px; color: #e3b341; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
  .agent-proc-pid { min-width: 3.5em; text-align: right; }
  .agent-panel-permission { color: #ff7b72; margin: 0 10px 6px; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .agent-btn-allow { color: #7ee787; border-color: rgba(126, 231, 135, 0.45); }
  .agent-btn-deny { color: #ff7b72; border-color: rgba(255, 123, 114, 0.45); }
  .agent-panel-hook { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: 0 10px 6px; }
  .agent-panel-actions { display: flex; gap: 4px; flex-shrink: 0; }
  .agent-btn { background: transparent; border: 1px solid var(--border-subtle); color: inherit; border-radius: 4px; padding: 2px 7px; cursor: pointer; }
  .agent-btn:hover { background: rgba(255, 255, 255, 0.06); }
  .agent-tabs { display: flex; gap: 2px; padding: 0 10px 6px; border-bottom: 1px solid var(--border-subtle); flex-wrap: wrap; }
  .agent-tab { background: transparent; border: 0; border-bottom: 2px solid transparent; color: var(--text-secondary); padding: 4px 8px; cursor: pointer; font-size: 11px; }
  .agent-tab:hover { color: inherit; }
  .agent-tab-active { color: #63b3ed; border-bottom-color: #63b3ed; }
  .agent-panel-feedback { padding: 4px 10px; color: #7ee787; }
  .agent-panel-error { padding: 4px 10px; color: #f85149; white-space: pre-wrap; }
  .agent-panel-body { flex: 1; min-height: 0; overflow: auto; padding: 8px 10px; display: flex; flex-direction: column; gap: 8px; }
  .agent-panel-hint { color: var(--text-secondary); margin: 0; }
  .agent-panel-warn { color: #e3b341; }
  .agent-session-pick { display: flex; flex-direction: column; gap: 4px; font-size: 11px; color: var(--text-secondary); }
  .agent-session-pick select { background: var(--bg-surface, #151928); color: inherit; border: 1px solid var(--border-subtle); border-radius: 4px; padding: 3px 6px; font-size: 11px; max-width: 100%; }
  .agent-summary { border: 1px solid rgba(99, 179, 237, 0.3); background: rgba(99, 179, 237, 0.06); border-radius: 6px; padding: 6px 8px; }
  .agent-summary-prompt { color: var(--text-secondary); font-style: italic; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; margin-bottom: 4px; }
  .agent-summary-text { white-space: pre-wrap; word-break: break-word; }
  .agent-group-label { color: var(--text-secondary); font-size: 10px; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px; }
  .agent-more { align-self: flex-start; }
  .agent-msg { border: 1px solid var(--border-subtle); border-radius: 6px; padding: 6px 8px; }
  .agent-msg-user { white-space: pre-wrap; word-break: break-word; opacity: 0.85; }
  .agent-msg-meta { display: flex; gap: 8px; align-items: center; color: var(--text-secondary); font-size: 11px; margin-bottom: 4px; flex-wrap: wrap; }
  .agent-msg-meta span:first-child { font-weight: 600; color: inherit; }
  .agent-link { background: none; border: 0; color: #63b3ed; cursor: pointer; padding: 0 2px; font-size: 11px; }
  .agent-link:hover { text-decoration: underline; }
  .agent-link:disabled { opacity: 0.4; cursor: default; text-decoration: none; }
  .agent-prose { word-break: break-word; }
  .agent-prose :global(p) { margin: 4px 0; }
  .agent-prose :global(ul), .agent-prose :global(ol) { margin: 4px 0; padding-left: 18px; }
  .agent-prose :global(code) { font-family: var(--font-mono); font-size: 11px; background: rgba(255, 255, 255, 0.06); padding: 0 3px; border-radius: 3px; }
  .agent-code { border: 1px solid var(--border-subtle); border-radius: 6px; overflow: hidden; }
  .agent-code-bar { display: flex; gap: 8px; align-items: center; padding: 3px 8px; background: rgba(255, 255, 255, 0.04); border-bottom: 1px solid var(--border-subtle); }
  .agent-code-lang { font-family: var(--font-mono); font-size: 10px; color: var(--text-secondary); margin-right: auto; }
  .agent-code pre, .agent-cmd pre, .agent-pre { margin: 0; padding: 8px; overflow-x: auto; font-family: var(--font-mono); font-size: 11px; line-height: 1.4; }
  .agent-pre { border: 1px solid var(--border-subtle); border-radius: 6px; white-space: pre; }
  .agent-pre-dim { color: var(--text-secondary); }
  .agent-inline { display: flex; align-items: center; gap: 8px; padding: 3px 8px; border: 1px solid var(--border-subtle); border-radius: 6px; }
  .agent-inline code { font-family: var(--font-mono); font-size: 11px; flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .agent-section-head { display: flex; justify-content: space-between; align-items: center; font-weight: 600; margin-top: 4px; }
  .agent-row { display: flex; align-items: center; gap: 6px; padding: 3px 0; border-bottom: 1px solid var(--border-subtle); }
  .agent-row-main { font-family: var(--font-mono); font-size: 11px; flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .agent-row-dim { color: var(--text-secondary); font-size: 10px; white-space: nowrap; }
  .agent-ops { display: inline-flex; gap: 2px; flex-shrink: 0; }
  .agent-op { font-size: 9px; text-transform: uppercase; border-radius: 3px; padding: 0 4px; border: 1px solid var(--border-subtle); color: var(--text-secondary); }
  .agent-op-write, .agent-op-edit { color: #e3b341; border-color: rgba(227, 179, 65, 0.4); }
  .agent-op-delete { color: #f85149; border-color: rgba(248, 81, 73, 0.4); }
  .agent-tasks { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
  .agent-task { display: flex; align-items: flex-start; gap: 8px; padding: 4px 8px; border: 1px solid var(--border-subtle); border-radius: 6px; }
  .agent-task-mark { font-family: var(--font-mono); width: 1em; flex-shrink: 0; }
  .agent-task-completed { opacity: 0.55; }
  .agent-task-completed .agent-task-text { text-decoration: line-through; }
  .agent-task-in_progress { border-color: rgba(227, 179, 65, 0.5); }
  .agent-task-in_progress .agent-task-mark { color: #e3b341; }
  .agent-task-completed .agent-task-mark { color: #7ee787; }
  .agent-task-prio { color: #f85149; font-weight: 700; margin-left: auto; }
  .agent-cmd { border: 1px solid var(--border-subtle); border-radius: 6px; padding: 4px 8px 0; }
  .agent-cmd-failed { border-color: rgba(248, 81, 73, 0.4); }
  .agent-exit { font-family: var(--font-mono); }
  .agent-exit-ok { color: #7ee787; }
  .agent-exit-fail { color: #f85149; }
  .agent-panel-footer { padding: 4px 10px; border-top: 1px solid var(--border-subtle); color: var(--text-secondary); font-size: 10px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-height: 14px; }
</style>
