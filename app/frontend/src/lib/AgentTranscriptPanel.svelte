<script>
  // Side panel showing the last exchanges of the coding agent running in a
  // terminal, read from the agent's own session file. Code blocks arrive
  // exactly as the agent wrote them, without terminal wrapping.
  import { onDestroy } from 'svelte';
  import { marked } from 'marked';
  import { ClipboardSetText } from '../../wailsjs/runtime';
  import { t } from './i18n.js';
  import { sanitizeHtml } from './util.js';
  import { splitMarkdown } from './agents/markdownBlocks.js';
  import { agentStates } from './stores/agentStore.js';
  import { terminalMap } from './stores/terminalStore.js';
  import { notesPanelOpen } from './stores/uiStore.js';
  import { closeAgentPanel, loadAgentPaneText, loadAgentTranscript } from './actions/agentActions.js';

  export let terminalId;

  const REFRESH_WORKING_MS = 6000;
  const MESSAGE_LIMIT = 12;

  let transcript = null;
  // 'file': messages from the agent's session file (exact text).
  // 'screen': the tmux pane capture (always available, but with the agent's
  // own line breaks). The user can switch to compare both.
  let view = 'file';
  let pane = null;
  let paneFull = false;
  let paneLoading = false;
  let loading = false;
  let error = '';
  let feedback = '';
  let feedbackTimer = null;
  let refreshTimer = null;
  let loadedFor = null;

  $: agent = $agentStates[terminalId] || null;
  $: term = $terminalMap.get(terminalId) || null;
  $: if (terminalId !== loadedFor) { loadedFor = terminalId; transcript = null; pane = null; paneFull = false; view = 'file'; error = ''; refresh(); }
  // A finished turn means new content: reload once the agent goes idle.
  $: if (agent?.status === 'idle' && agent?.lastChange) refreshSoon();
  $: scheduleAutoRefresh(agent?.status);

  function scheduleAutoRefresh(status) {
    if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null; }
    if (status === 'working') refreshTimer = setInterval(refresh, REFRESH_WORKING_MS);
  }

  let refreshSoonTimer = null;
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

  function showView(next) {
    view = next;
    if (next === 'screen' && !pane) refreshPane();
    if (next === 'file' && !transcript) refresh();
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
    const xterm = term?.terminal;
    if (!xterm || typeof xterm.paste !== 'function') return;
    // paste() applies bracketed paste, so multi-line snippets are not executed
    // line by line; the user still confirms with Enter.
    xterm.paste(code.replace(/\n+$/, ''));
    xterm.focus?.();
    flash($t('agentPanel.inserted'));
  }

  async function saveToNotes(block) {
    const stamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
    const filename = `agent-${agent?.kind || 'snippet'}-${stamp}.md`;
    const body = '```' + (block.lang || '') + '\n' + block.code + '\n```\n';
    try {
      await window['go']['main']['App']['SaveNote'](filename, body);
      notesPanelOpen.set(true);
      flash($t('agentPanel.savedToNotes', { filename }));
    } catch (e) {
      error = `Save failed: ${e?.message || e}`;
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

  onDestroy(() => {
    if (refreshTimer) clearInterval(refreshTimer);
    if (refreshSoonTimer) clearTimeout(refreshSoonTimer);
    if (feedbackTimer) clearTimeout(feedbackTimer);
  });
</script>

<div class="agent-panel-inner">
  <div class="agent-panel-header">
    <div class="agent-panel-title">
      <span class="agent-panel-label">{agent?.label || $t('agentPanel.title')}</span>
      {#if agent?.status && agent.status !== 'unknown'}
        <span class="agent-panel-status agent-status-{agent.status}">{agent.status === 'working' ? $t('agentPanel.working') : $t('agentPanel.idle')}</span>
      {/if}
      <span class="agent-panel-sub" title={transcript?.cwd || agent?.cwd || ''}>{term?.name || ''}{agent?.cwd ? ' · ' + shortPath(agent.cwd) : ''}</span>
    </div>
    <div class="agent-panel-actions">
      <div class="agent-view-switch" role="tablist">
        <button type="button" class="agent-btn {view === 'file' ? 'agent-btn-active' : ''}" role="tab" aria-selected={view === 'file'} on:click={() => showView('file')} title={$t('agentPanel.viewFileTitle')}>{$t('agentPanel.viewFile')}</button>
        <button type="button" class="agent-btn {view === 'screen' ? 'agent-btn-active' : ''}" role="tab" aria-selected={view === 'screen'} on:click={() => showView('screen')} title={$t('agentPanel.viewScreenTitle')}>{$t('agentPanel.viewScreen')}</button>
      </div>
      <button type="button" class="agent-btn" on:click={refresh} disabled={loading || paneLoading} title={$t('agentPanel.refresh')}>{loading || paneLoading ? '…' : '↻'}</button>
      <button type="button" class="agent-btn" on:click={closeAgentPanel} title={$t('agentPanel.close')}>&#x2715;</button>
    </div>
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
      {:else}
        <p class="agent-panel-hint {transcript.verified ? 'agent-panel-ok' : 'agent-panel-warn'}" title={transcript.sessionFile}>
          {transcript.verified ? $t('agentPanel.verified') : $t('agentPanel.unverified')}{#if transcript.candidates > 1} · {$t('agentPanel.candidates', { n: transcript.candidates })}{/if}
        </p>
      {/if}
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
                    <button type="button" class="agent-link" on:click={() => insertIntoTerminal(segment.code)} disabled={!term}>{$t('agentPanel.insert')}</button>
                    <button type="button" class="agent-link" on:click={() => saveToNotes(segment)}>{$t('agentPanel.toNotes')}</button>
                  </div>
                  <pre><code>{segment.code}</code></pre>
                </div>
              {/if}
            {/each}
          {/if}
        </div>
      {/each}
    {/if}
  </div>
  <div class="agent-panel-footer" title={transcript?.sessionFile || ''}>
    {#if transcript?.sessionFile}{$t('agentPanel.source')}: {shortPath(transcript.sessionFile)}{/if}
  </div>
</div>

<style>
  .agent-panel-inner { display: flex; flex-direction: column; height: 100%; min-height: 0; font-size: 12px; }
  .agent-panel-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; padding: 8px 10px; border-bottom: 1px solid var(--border-subtle); }
  .agent-panel-title { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; min-width: 0; }
  .agent-panel-label { font-weight: 700; }
  .agent-panel-sub { color: var(--text-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
  .agent-panel-status { border-radius: 999px; padding: 1px 7px; font-size: 10px; font-weight: 700; text-transform: lowercase; border: 1px solid transparent; }
  .agent-status-working { background: rgba(227, 179, 65, 0.14); color: #e3b341; border-color: rgba(227, 179, 65, 0.32); }
  .agent-status-idle { background: rgba(126, 231, 135, 0.14); color: #7ee787; border-color: rgba(126, 231, 135, 0.28); }
  .agent-panel-actions { display: flex; gap: 4px; flex-shrink: 0; }
  .agent-btn { background: transparent; border: 1px solid var(--border-subtle); color: inherit; border-radius: 4px; padding: 2px 7px; cursor: pointer; }
  .agent-btn:hover { background: rgba(255, 255, 255, 0.06); }
  .agent-btn-active { background: rgba(99, 179, 237, 0.18); border-color: rgba(99, 179, 237, 0.5); }
  .agent-view-switch { display: flex; gap: 2px; margin-right: 4px; }
  .agent-panel-ok { color: #7ee787; }
  .agent-panel-warn { color: #e3b341; }
  .agent-panel-feedback { padding: 4px 10px; color: #7ee787; }
  .agent-panel-error { padding: 4px 10px; color: #f85149; white-space: pre-wrap; }
  .agent-panel-body { flex: 1; min-height: 0; overflow: auto; padding: 8px 10px; display: flex; flex-direction: column; gap: 10px; }
  .agent-panel-hint { color: var(--text-secondary); margin: 0; }
  .agent-msg { border: 1px solid var(--border-subtle); border-radius: 6px; padding: 6px 8px; }
  .agent-msg-user { white-space: pre-wrap; word-break: break-word; opacity: 0.85; }
  .agent-msg-meta { display: flex; gap: 8px; align-items: center; color: var(--text-secondary); font-size: 11px; margin-bottom: 4px; }
  .agent-msg-meta span:first-child { font-weight: 600; color: inherit; }
  .agent-link { background: none; border: 0; color: #63b3ed; cursor: pointer; padding: 0 2px; font-size: 11px; }
  .agent-link:hover { text-decoration: underline; }
  .agent-link:disabled { opacity: 0.4; cursor: default; text-decoration: none; }
  .agent-prose { word-break: break-word; }
  .agent-prose :global(p) { margin: 4px 0; }
  .agent-prose :global(ul), .agent-prose :global(ol) { margin: 4px 0; padding-left: 18px; }
  .agent-prose :global(code) { font-family: var(--font-mono); font-size: 11px; background: rgba(255, 255, 255, 0.06); padding: 0 3px; border-radius: 3px; }
  .agent-code { margin: 6px 0; border: 1px solid var(--border-subtle); border-radius: 6px; overflow: hidden; }
  .agent-code-bar { display: flex; gap: 8px; align-items: center; padding: 3px 8px; background: rgba(255, 255, 255, 0.04); border-bottom: 1px solid var(--border-subtle); }
  .agent-code-lang { font-family: var(--font-mono); font-size: 10px; color: var(--text-secondary); margin-right: auto; }
  .agent-code pre { margin: 0; padding: 8px; overflow-x: auto; font-family: var(--font-mono); font-size: 11px; line-height: 1.4; }
  .agent-panel-footer { padding: 4px 10px; border-top: 1px solid var(--border-subtle); color: var(--text-secondary); font-size: 10px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-height: 14px; }
</style>
