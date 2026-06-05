<script>
  import { createEventDispatcher, tick } from 'svelte';
  import { t } from './i18n.js';

  export let node;
  export let terminalMap;
  export let activeTerminalId;

  const dispatch = createEventDispatcher();

  let containerEl;
  let isDragging = false;

  $: localRatio = node.type === 'split' ? node.ratio : 0.5;

  function startDrag(e) {
    isDragging = true;
    e.preventDefault();

    const onMouseMove = (ev) => {
      if (!isDragging || !containerEl) return;
      const rect = containerEl.getBoundingClientRect();

      if (node.direction === 'horizontal') {
        localRatio = Math.max(0.1, Math.min(0.9, (ev.clientX - rect.left) / rect.width));
      } else {
        localRatio = Math.max(0.1, Math.min(0.9, (ev.clientY - rect.top) / rect.height));
      }

      node.ratio = localRatio;
      dispatch('ratiochange');
    };

    const onMouseUp = () => {
      isDragging = false;
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
      dispatch('resize');
    };

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  }

  function handleSearchKeydown(e, termId) {
    if (e.key === 'Enter' && e.shiftKey) {
      e.preventDefault();
      dispatch('searchprev', termId);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      dispatch('searchnext', termId);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      dispatch('searchclose', termId);
    }
  }

  function tmuxTitle(term) {
    const status = term.tmuxStatus || (term.tmuxActive ? 'active' : 'plain');
    const parts = [`tmux ${status}`];
    if (term.tmuxSessionName) parts.push(term.tmuxSessionName);
    if (term.shellPath) parts.push(term.shellPath);
    if (term.tmuxError) parts.push(term.tmuxError);
    return parts.join(' · ');
  }

  function rcTitle(term) {
    if (!term.rcMode || term.rcMode === 'off') return $t('splitPane.noRcApplied');
    return $t('splitPane.rcModeTitle', { mode: term.rcMode });
  }
</script>

{#if node.type === 'leaf'}
  {@const term = terminalMap.get(node.terminalId)}
  {#if term && !term.minimized}
    <div
      class="terminal-wrapper {node.terminalId === activeTerminalId ? 'active-terminal' : ''}"
      on:click={() => dispatch('activate', node.terminalId)}
      on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') dispatch('activate', node.terminalId); }}
      tabindex="0"
      role="button"
    >
      <div
        class="terminal-header"
        role="button"
        tabindex="0"
        draggable="true"
        on:dragstart={(e) => dispatch('dragstart', { event: e, id: node.terminalId })}
        on:dragover={(e) => dispatch('dragover', { event: e, id: node.terminalId })}
        on:dragleave={(e) => dispatch('dragleave', { event: e })}
        on:drop={(e) => dispatch('drop', { event: e, id: node.terminalId })}
        on:dragend={(e) => dispatch('dragend', { event: e })}
      >
        <div class="header-left">
          <span class="terminal-type-badge">
            {term.type?.toUpperCase() || ''}{#if term.disconnected}<span class="disconnect-dot"></span>{/if}
          </span>
          {#if term.tmuxActive || ['failed', 'missing'].includes(term.tmuxStatus)}
            <span class="tmux-badge {term.tmuxStatus === 'failed' || term.tmuxStatus === 'missing' ? 'tmux-badge-warning' : ''}" title={tmuxTitle(term)}>{term.tmuxStatus === 'missing' ? 'no tx' : 'tx'}</span>
          {/if}
          {#if term.type === 'ssh'}
            <span class="rc-badge {term.rcMode && term.rcMode !== 'off' ? 'rc-badge-active' : ''}" title={rcTitle(term)}>{term.rcMode && term.rcMode !== 'off' ? 'rc' : 'clean'}</span>
          {/if}
          {#if term.restoreClass && term.restoreClass !== 'fresh'}
            <span class="restore-badge restore-{term.restoreClass}">
              {#if term.restoreClass === 'live-restored'}
                Live Restored
              {:else if term.restoreClass === 'rehydrated'}
                Rehydrated
              {:else if term.restoreClass === 'transcript-restored'}
                Transcript
              {:else}
                Restored
              {/if}
            </span>
          {/if}
          {#if term.editingName}
            <input
              type="text"
              id="terminal-name-input-{term.id}"
              value={term.name}
              on:blur={(e) => dispatch('savename', { id: term.id, event: e })}
              on:keydown={(e) => { if (e.key === 'Enter') e.target.blur(); }}
              class="name-input"
            />
          {:else}
            <span
              class="terminal-name"
              on:click|stopPropagation={() => dispatch('editname', term.id)}
              on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') dispatch('editname', term.id); }}
              tabindex="0"
              role="button"
            >{term.name}</span>
          {/if}
        </div>
        <div class="header-controls">
          <button class="header-btn record-btn" class:recording={term.recording}
            on:click|stopPropagation={() => dispatch('togglerecording', node.terminalId)}
            title={term.recording ? $t('splitPane.stopRecording') : $t('splitPane.startRecording')}>&#x23FA;</button>
          <button class="header-btn split-btn" on:click|stopPropagation={() => dispatch('split', { id: node.terminalId, direction: 'horizontal' })} title={$t('splitPane.splitRight')}>
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none"><rect x="0.5" y="0.5" width="11" height="11" rx="1" stroke="currentColor" stroke-width="1"/><line x1="6" y1="1" x2="6" y2="11" stroke="currentColor" stroke-width="1"/></svg>
          </button>
          <button class="header-btn split-btn" on:click|stopPropagation={() => dispatch('split', { id: node.terminalId, direction: 'vertical' })} title={$t('splitPane.splitDown')}>
            <svg width="12" height="12" viewBox="0 0 12 12" fill="none"><rect x="0.5" y="0.5" width="11" height="11" rx="1" stroke="currentColor" stroke-width="1"/><line x1="1" y1="6" x2="11" y2="6" stroke="currentColor" stroke-width="1"/></svg>
          </button>
          <span class="header-sep"></span>
          <button class="header-btn minimize-btn" on:click|stopPropagation={() => dispatch('minimize', node.terminalId)} title={$t('splitPane.minimize')}>&#x2013;</button>
          <button class="header-btn close-btn" on:click|stopPropagation={() => dispatch('close', node.terminalId)} title={$t('splitPane.close')}>&#x2715;</button>
        </div>
      </div>
      <div class="terminal-container">
        <div id="terminal-{term.id}" class="terminal"></div>
        {#if term.restoredTranscript && term.restoreClass === 'transcript-restored' && !term.restoreDismissed}
          <div
            class="restore-summary"
            role="dialog"
            tabindex="-1"
            on:keydown|stopPropagation
            on:mousedown|stopPropagation
            on:pointerdown|stopPropagation
            on:click|stopPropagation
          >
            <div class="restore-summary-header">
              <div class="restore-summary-title">{$t('splitPane.restoredTranscript')}</div>
              <button
                type="button"
                class="restore-summary-close"
                on:mousedown|stopPropagation
                on:pointerdown|stopPropagation
                on:click|stopPropagation={() => dispatch('dismissrestore', term.id)}
                title={$t('splitPane.closeRestored')}
              >
                ×
              </button>
            </div>
            <pre>{term.restoredTranscript}</pre>
          </div>
        {/if}
        {#if term.searchVisible}
          <div class="search-bar" role="toolbar" tabindex="-1" on:click|stopPropagation on:keydown|stopPropagation>
            <input
              type="text"
              class="search-input"
              placeholder={$t('splitPane.searchPlaceholder')}
              value={term.searchQuery}
              on:input={(e) => dispatch('searchinput', { id: term.id, query: e.target.value })}
              on:keydown={(e) => handleSearchKeydown(e, term.id)}
            />
            <button class="search-btn" on:click={() => dispatch('searchprev', term.id)} title={$t('splitPane.searchPrev')}>&#x25B2;</button>
            <button class="search-btn" on:click={() => dispatch('searchnext', term.id)} title={$t('splitPane.searchNext')}>&#x25BC;</button>
            <button class="search-btn search-close-btn" on:click={() => dispatch('searchclose', term.id)} title={$t('splitPane.searchClose')}>&#x2715;</button>
          </div>
        {/if}
        {#if term.disconnected}
          <div class="disconnect-overlay">
            <p>{$t('splitPane.connectionLost')}</p>
            <button on:click|stopPropagation={() => dispatch('reconnect', term.id)} disabled={term.reconnecting}>
              {term.reconnecting ? 'Reconnecting...' : 'Reconnect'}
            </button>
          </div>
        {/if}
      </div>
    </div>
  {/if}

{:else if node.type === 'split'}
  <div
    class="split-container split-{node.direction}"
    bind:this={containerEl}
  >
    <div class="split-pane" style="flex: {localRatio} 1 0px">
      <svelte:self
        node={node.children[0]}
        {terminalMap}
        {activeTerminalId}
        on:activate
        on:split
        on:minimize
        on:close
        on:reconnect
        on:editname
        on:savename
        on:resize
        on:ratiochange
        on:dragstart
        on:dragover
        on:dragleave
        on:drop
        on:dragend
        on:searchinput
        on:searchnext
        on:searchprev
        on:searchclose
        on:dismissrestore
        on:togglerecording
      />
    </div>

    <div
      class="split-divider split-divider-{node.direction}"
      on:mousedown={startDrag}
      role="slider"
      tabindex="0"
      aria-orientation={node.direction === 'horizontal' ? 'vertical' : 'horizontal'}
      aria-valuemin="10"
      aria-valuemax="90"
      aria-valuenow={Math.round(localRatio * 100)}
    ></div>

    <div class="split-pane" style="flex: {1 - localRatio} 1 0px">
      <svelte:self
        node={node.children[1]}
        {terminalMap}
        {activeTerminalId}
        on:activate
        on:split
        on:minimize
        on:close
        on:reconnect
        on:editname
        on:savename
        on:resize
        on:ratiochange
        on:dragstart
        on:dragover
        on:dragleave
        on:drop
        on:dragend
        on:searchinput
        on:searchnext
        on:searchprev
        on:searchclose
        on:dismissrestore
        on:togglerecording
      />
    </div>
  </div>
{/if}

<style>
  .split-container {
    display: flex;
    width: 100%;
    height: 100%;
    overflow: hidden;
  }

  .split-horizontal {
    flex-direction: row;
  }

  .split-vertical {
    flex-direction: column;
  }

  .split-pane {
    overflow: hidden;
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }

  .split-divider {
    flex-shrink: 0;
    background: var(--border-subtle);
    transition: background 150ms ease;
    z-index: 1;
  }

  .split-divider:hover {
    background: var(--accent);
  }

  .split-divider-horizontal {
    width: 4px;
    cursor: col-resize;
  }

  .split-divider-vertical {
    height: 4px;
    cursor: row-resize;
  }

  .terminal-container {
    position: relative;
    flex: 1;
    min-height: 0;
    width: 100%;
    height: 100%;
    overflow: hidden;
  }

  .restore-badge {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 2px 8px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.01em;
    margin-left: 8px;
  }

  .tmux-badge {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 2px 7px;
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.03em;
    margin-left: 6px;
    background: rgba(126, 231, 135, 0.14);
    color: #7ee787;
    border: 1px solid rgba(126, 231, 135, 0.28);
    font-family: var(--font-mono);
    text-transform: lowercase;
  }

  .tmux-badge-warning {
    background: rgba(227, 179, 65, 0.14);
    color: #e3b341;
    border-color: rgba(227, 179, 65, 0.32);
  }

  .rc-badge {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 2px 7px;
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.03em;
    margin-left: 6px;
    background: rgba(139, 148, 158, 0.12);
    color: #8b949e;
    border: 1px solid rgba(139, 148, 158, 0.24);
    font-family: var(--font-mono);
    text-transform: lowercase;
  }

  .rc-badge-active {
    background: rgba(99, 179, 237, 0.14);
    color: #79c0ff;
    border-color: rgba(99, 179, 237, 0.3);
  }

  .restore-live-restored {
    background: rgba(126, 231, 135, 0.14);
    color: #7ee787;
  }

  .restore-rehydrated {
    background: rgba(227, 179, 65, 0.14);
    color: #e3b341;
  }

  .restore-transcript-restored {
    background: rgba(99, 179, 237, 0.14);
    color: #63b3ed;
  }

  .restore-summary {
    position: absolute;
    top: 10px;
    right: 10px;
    z-index: 60;
    pointer-events: auto;
    border: 1px solid rgba(99, 179, 237, 0.2);
    background: rgba(12, 14, 20, 0.92);
    border-radius: 8px;
    padding: 10px 12px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
    width: min(420px, calc(100% - 20px));
    max-height: 140px;
    overflow: auto;
  }

  .restore-summary-title {
    color: #63b3ed;
    font-size: 11px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 8px;
  }

  .restore-summary-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    margin-bottom: 8px;
    pointer-events: auto;
  }

  .restore-summary-close {
    position: relative;
    z-index: 61;
    pointer-events: auto;
    border: none;
    background: transparent;
    color: #8b949e;
    cursor: pointer;
    font-size: 16px;
    line-height: 1;
    padding: 0;
  }

  .restore-summary pre {
    margin: 0;
    font-size: 12px;
    line-height: 1.4;
    color: #d7dee9;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .restore-summary-close:hover {
    color: #f0f3f6;
  }

  .restore-summary pre {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    color: #c9d1d9;
    font-size: 11px;
    line-height: 1.45;
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
  }

  /* ─── Search Bar ─────────────────────────────────────── */

  .search-bar {
    position: absolute;
    top: 4px;
    right: 8px;
    display: flex;
    align-items: center;
    gap: 2px;
    background: #1a1e2e;
    border: 1px solid #2d3348;
    border-radius: 4px;
    padding: 3px 4px;
    z-index: 20;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
  }

  .search-input {
    background: #0c0e14;
    border: 1px solid #2d3348;
    border-radius: 3px;
    color: #c9d1d9;
    font-family: 'JetBrains Mono', 'Fira Code', monospace;
    font-size: 12px;
    padding: 3px 8px;
    width: 180px;
    outline: none;
  }

  .search-input:focus {
    border-color: #63b3ed;
  }

  .search-input::placeholder {
    color: #545d68;
  }

  .search-btn {
    background: transparent;
    border: none;
    color: #8b949e;
    cursor: pointer;
    font-size: 10px;
    padding: 3px 5px;
    border-radius: 3px;
    line-height: 1;
    transition: background 150ms ease, color 150ms ease;
  }

  .search-btn:hover {
    background: #2d3348;
    color: #c9d1d9;
  }

  .search-close-btn:hover {
    color: #f47067;
  }

  /* ─── Disconnect Overlay ─────────────────────────────── */

  .disconnect-overlay {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    background: rgba(12, 14, 20, 0.85);
    z-index: 10;
    gap: 12px;
  }

  .disconnect-overlay p {
    color: #f47067;
    font-size: 14px;
    font-weight: 600;
    margin: 0;
  }

  .disconnect-overlay button {
    padding: 6px 18px;
    border: 1px solid #63b3ed;
    border-radius: 4px;
    background: transparent;
    color: #63b3ed;
    font-size: 13px;
    cursor: pointer;
    transition: background 150ms ease, color 150ms ease;
  }

  .disconnect-overlay button:hover:not(:disabled) {
    background: #63b3ed;
    color: #0c0e14;
  }

  .disconnect-overlay button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .disconnect-dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: #f47067;
    margin-left: 4px;
    vertical-align: middle;
  }

  .record-btn {
    font-size: 10px;
    color: #565f89;
    transition: color 0.2s;
  }

  .record-btn:hover {
    color: #e53e3e;
  }

  .record-btn.recording {
    color: #e53e3e;
    animation: pulse-record 1.5s infinite;
  }

  @keyframes pulse-record {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
</style>
