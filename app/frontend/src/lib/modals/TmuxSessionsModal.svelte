<script>
  // Overview of every tmux session on the connected SSH hosts and on
  // Mimir's local tmux server. Shows what runs where, and lets the user
  // open a session that lost its pane or end it. See
  // actions/tmuxSessionActions.js for the rules.
  import { onMount } from 'svelte';
  import { modalFocus } from '../actions/modalFocus.js';
  import { t } from '../i18n.js';
  import {
    canKillSession,
    canOpenSession,
    killTmuxSession,
    loadTmuxSessions,
    openTmuxSession,
    orphanCount,
    sessionAge,
    sessionStatus,
    showSessionPane,
  } from '../actions/tmuxSessionActions.js';

  export let onClose = () => {};

  let groups = [];
  let loading = true;
  let loadError = '';
  let busy = '';        // key of the session an action is running for
  let confirmKill = ''; // key of the session whose "end" needs a second click

  const keyOf = (group, row) => `${group.kind}:${group.profileId || ''}:${row.name}`;

  async function refresh() {
    loading = true;
    loadError = '';
    confirmKill = '';
    try {
      groups = await loadTmuxSessions();
    } catch (error) {
      loadError = String(error?.message || error);
      groups = [];
    }
    loading = false;
  }

  onMount(refresh);

  async function open(group, row) {
    busy = keyOf(group, row);
    await openTmuxSession(group, row);
    busy = '';
  }

  async function kill(group, row) {
    const key = keyOf(group, row);
    if (confirmKill !== key) {
      confirmKill = key;
      return;
    }
    busy = key;
    const done = await killTmuxSession(group, row);
    busy = '';
    if (done) await refresh();
  }

  $: orphans = orphanCount(groups);
</script>

<div class="modal-overlay" on:click|self={onClose} on:keydown={(e) => { if (e.key === 'Escape') onClose(); }} role="presentation">
  <div class="tmux-sessions-modal" role="dialog" use:modalFocus={{ onEscape: onClose }} aria-modal="true" tabindex="-1">
    <div class="template-prompt-header">
      <h3>{$t('tmuxSessions.title')}</h3>
      <div class="tmux-sessions-head-actions">
        <button type="button" class="modal-secondary-button" disabled={loading} on:click={refresh}>{$t('tmuxSessions.refresh')}</button>
        <button type="button" class="modal-close-button" on:click={onClose}>&#x2715;</button>
      </div>
    </div>
    <p class="template-prompt-text">{$t('tmuxSessions.intro')}</p>

    {#if loading}
      <p class="tmux-sessions-note">{$t('tmuxSessions.loading')}</p>
    {:else if loadError}
      <p class="tmux-sessions-note tmux-sessions-error">{loadError}</p>
    {:else if groups.length === 0}
      <p class="tmux-sessions-note">{$t('tmuxSessions.empty')}</p>
    {:else}
      {#if orphans > 0}
        <p class="tmux-sessions-note tmux-sessions-orphans">{$t('tmuxSessions.orphanSummary', { count: orphans })}</p>
      {/if}
      {#each groups as group (group.kind + ':' + (group.profileId || ''))}
        <section class="tmux-sessions-group">
          <h4>
            {group.kind === 'ssh' ? `SSH: ${group.label}` : $t('tmuxSessions.local')}
            <small>{group.sessions.length}</small>
          </h4>
          {#if group.error}
            <p class="tmux-sessions-note tmux-sessions-error">{group.error}</p>
          {:else if group.sessions.length === 0}
            <p class="tmux-sessions-note">{$t('tmuxSessions.groupEmpty')}</p>
          {:else}
            <ul class="tmux-sessions-list">
              {#each group.sessions as row (row.name)}
                {@const status = sessionStatus(row)}
                {@const key = keyOf(group, row)}
                <li class="tmux-session" class:tmux-session-orphan={status === 'orphan'}>
                  <div class="tmux-session-main">
                    <div class="tmux-session-top">
                      <strong class="tmux-session-command">{row.command || '?'}</strong>
                      <span class="tmux-session-badge tmux-session-badge-{status}">{$t(`tmuxSessions.status.${status}`)}</span>
                      {#if !row.mimir}<span class="tmux-session-badge">{$t('tmuxSessions.foreign')}</span>{/if}
                    </div>
                    <div class="tmux-session-path" title={row.path}>{row.path}</div>
                    <div class="tmux-session-meta">
                      {row.name}
                      {#if row.created} · {$t('tmuxSessions.age', { age: sessionAge(row.created) })}{/if}
                      {#if row.windows > 1} · {$t('tmuxSessions.windows', { count: row.windows })}{/if}
                    </div>
                  </div>
                  <div class="tmux-session-actions">
                    {#if status === 'pane'}
                      <button type="button" class="modal-secondary-button" on:click={() => showSessionPane(row)}>{$t('tmuxSessions.show')}</button>
                    {/if}
                    {#if canOpenSession(row)}
                      <button
                        type="button"
                        class="modal-primary-button"
                        disabled={busy === key}
                        title={status === 'attached' ? $t('tmuxSessions.openAttachedTitle') : ''}
                        on:click={() => open(group, row)}
                      >{$t('tmuxSessions.open')}</button>
                    {/if}
                    {#if canKillSession(row)}
                      <button
                        type="button"
                        class="modal-secondary-button tmux-session-kill"
                        class:tmux-session-kill-armed={confirmKill === key}
                        disabled={busy === key}
                        title={$t('tmuxSessions.killTitle')}
                        on:click={() => kill(group, row)}
                      >{confirmKill === key ? $t('tmuxSessions.killConfirm') : $t('tmuxSessions.kill')}</button>
                    {/if}
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/each}
    {/if}
    <p class="tmux-sessions-hint">{$t('tmuxSessions.hint')}</p>
  </div>
</div>

<style>
  .tmux-sessions-modal {
    width: min(820px, 94vw);
    max-height: 86vh;
    overflow: auto;
    background: var(--bg-deep);
    border: 1px solid var(--border-accent);
    border-radius: var(--radius-lg);
    box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
    padding: 1.2rem;
  }

  .tmux-sessions-head-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .tmux-sessions-group h4 {
    margin: 1rem 0 0.4rem;
    font-size: 0.85rem;
    color: var(--text-primary);
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
  }

  .tmux-sessions-group h4 small {
    color: var(--text-secondary);
    font-weight: 400;
  }

  .tmux-sessions-note,
  .tmux-sessions-hint {
    font-size: 0.78rem;
    color: var(--text-secondary);
    margin: 0.4rem 0;
  }

  .tmux-sessions-hint {
    margin-top: 1rem;
  }

  .tmux-sessions-error {
    color: var(--danger, #e5534b);
  }

  .tmux-sessions-orphans {
    color: var(--warning, #d29922);
  }

  .tmux-sessions-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .tmux-session {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.8rem;
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--border-dim);
    border-radius: var(--radius-sm);
  }

  .tmux-session-orphan {
    border-color: var(--warning, #d29922);
  }

  .tmux-session-main {
    min-width: 0;
    flex: 1;
  }

  .tmux-session-top {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .tmux-session-command {
    font-family: var(--font-mono, monospace);
    font-size: 0.85rem;
    color: var(--text-primary);
  }

  .tmux-session-badge {
    font-size: 0.68rem;
    padding: 0.05rem 0.4rem;
    border: 1px solid var(--border-dim);
    border-radius: 999px;
    color: var(--text-secondary);
  }

  .tmux-session-badge-orphan {
    border-color: var(--warning, #d29922);
    color: var(--warning, #d29922);
  }

  .tmux-session-badge-pane {
    border-color: var(--accent);
    color: var(--accent);
  }

  .tmux-session-path,
  .tmux-session-meta {
    font-size: 0.74rem;
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tmux-session-path {
    font-family: var(--font-mono, monospace);
  }

  .tmux-session-actions {
    display: flex;
    gap: 0.4rem;
    flex-shrink: 0;
  }

  .tmux-session-actions button {
    padding: 0.35rem 0.7rem;
    font-size: 0.78rem;
  }

  .tmux-session-kill-armed {
    border-color: var(--danger, #e5534b);
    color: var(--danger, #e5534b);
  }
</style>
