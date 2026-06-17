<script>
  // Secure .env viewer. Reads the .env file from the terminal's working
  // directory over the same out-of-band channel discovery uses (SSH exec or a
  // local tmux cwd read) — never through the shell, so the contents never touch
  // scrollback, recording or command history. Values are masked by default and
  // only revealed on explicit click; they are never logged.
  import { onDestroy } from 'svelte';
  import { t } from '../i18n.js';
  import { ClipboardSetText } from '../../../wailsjs/runtime';
  import { IsDotEnvViewerEnabled, SetDotEnvViewer, ReadDotEnvForTerminalJSON } from '../../../wailsjs/go/main/App';

  export let terminalId;
  export let terminalType = '';
  export let label = '';
  export let onClose = () => {};

  // 'checking' | 'consent' | 'loading' | 'ready' | 'error'
  let phase = 'checking';
  let entries = [];
  let source = '';
  let dir = '';
  let errorText = '';
  let enabling = false;

  // Per-row reveal state and transient "copied" feedback, keyed by index.
  let revealed = new Set();
  let copiedIndex = -1;
  let copiedTimer = null;

  init();

  async function init() {
    try {
      const enabled = await IsDotEnvViewerEnabled();
      if (!enabled) {
        phase = 'consent';
        return;
      }
      await load();
    } catch (err) {
      fail(err);
    }
  }

  async function enable() {
    enabling = true;
    try {
      await SetDotEnvViewer(true);
      await load();
    } catch (err) {
      fail(err);
    } finally {
      enabling = false;
    }
  }

  async function load() {
    phase = 'loading';
    revealed = new Set();
    try {
      const raw = await ReadDotEnvForTerminalJSON(terminalId, terminalType || '');
      const parsed = JSON.parse(raw);
      entries = Array.isArray(parsed.entries) ? parsed.entries : [];
      source = parsed.source || '';
      dir = parsed.dir || '';
      phase = 'ready';
    } catch (err) {
      fail(err);
    }
  }

  function fail(err) {
    // Only the error message is surfaced — never any file content.
    errorText = (err && err.message) ? err.message : String(err);
    phase = 'error';
  }

  function toggleReveal(index) {
    const next = new Set(revealed);
    if (next.has(index)) next.delete(index);
    else next.add(index);
    revealed = next;
  }

  function revealAll() {
    revealed = new Set(entries.map((_, i) => i));
  }

  function hideAll() {
    revealed = new Set();
  }

  async function copyValue(index) {
    try {
      await ClipboardSetText(entries[index]?.value ?? '');
      copiedIndex = index;
      clearTimeout(copiedTimer);
      copiedTimer = setTimeout(() => { copiedIndex = -1; }, 1500);
    } catch {
      // Clipboard failures are non-fatal and intentionally not logged with content.
    }
  }

  // Fixed-width mask: a length-dependent mask would leak how long each secret
  // is to a shoulder-surfer. Empty values render as a muted placeholder instead.
  function mask(value) {
    return value ? '••••••••' : '—';
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') onClose();
  }

  onDestroy(() => {
    // Drop revealed state and clear the parsed values from memory on close.
    clearTimeout(copiedTimer);
    revealed = new Set();
    entries = [];
  });
</script>

<div class="modal-overlay" on:click={onClose} on:keydown={handleKeydown} tabindex="0" role="button">
  <div class="dotenv-modal" role="dialog" aria-modal="true" tabindex="-1" on:click|stopPropagation on:keydown|stopPropagation>
    <div class="dotenv-header">
      <h3>
        {$t('dotEnvViewer.title')}
        {#if label}<span class="dotenv-target">· {label}</span>{/if}
      </h3>
      <button type="button" class="modal-close-button" on:click={onClose} aria-label={$t('dotEnvViewer.close')}>&#x2715;</button>
    </div>

    {#if phase === 'consent'}
      <div class="dotenv-consent">
        <p class="dotenv-intro">{$t('dotEnvViewer.intro')}</p>
        <p class="dotenv-security">{$t('dotEnvViewer.securityNote')}</p>
        <div class="dotenv-actions">
          <button type="button" class="modal-secondary-button" on:click={onClose}>{$t('dotEnvViewer.cancel')}</button>
          <button type="button" class="modal-primary-button" disabled={enabling} on:click={enable}>
            {enabling ? $t('dotEnvViewer.enabling') : $t('dotEnvViewer.enable')}
          </button>
        </div>
      </div>
    {:else if phase === 'checking' || phase === 'loading'}
      <div class="dotenv-state">{$t('dotEnvViewer.loading')}</div>
    {:else if phase === 'error'}
      <div class="dotenv-state dotenv-error">{errorText}</div>
      <div class="dotenv-actions">
        <button type="button" class="modal-secondary-button" on:click={onClose}>{$t('dotEnvViewer.close')}</button>
        <button type="button" class="modal-primary-button" on:click={load}>{$t('dotEnvViewer.retry')}</button>
      </div>
    {:else if phase === 'ready'}
      <div class="dotenv-subhead">
        <span class="dotenv-source">
          {#if source === 'remote'}
            {$t('dotEnvViewer.sourceRemote')}
          {:else if dir}
            {$t('dotEnvViewer.sourceLocal', { dir })}
          {/if}
        </span>
        {#if entries.length}
          <div class="dotenv-bulk">
            <button type="button" class="dotenv-link" on:click={revealAll}>{$t('dotEnvViewer.revealAll')}</button>
            <button type="button" class="dotenv-link" on:click={hideAll}>{$t('dotEnvViewer.hideAll')}</button>
          </div>
        {/if}
      </div>

      {#if entries.length === 0}
        <div class="dotenv-state">{$t('dotEnvViewer.empty')}</div>
      {:else}
        <div class="dotenv-list">
          {#each entries as entry, i}
            <div class="dotenv-row">
              <span class="dotenv-key" title={entry.key}>{entry.key}</span>
              <code class="dotenv-value" class:revealed={revealed.has(i)}>
                {revealed.has(i) ? (entry.value || '') : mask(entry.value)}
              </code>
              <div class="dotenv-row-actions">
                <button type="button" class="dotenv-icon-btn" on:click={() => toggleReveal(i)}
                  title={revealed.has(i) ? $t('dotEnvViewer.hide') : $t('dotEnvViewer.reveal')}
                  aria-label={revealed.has(i) ? $t('dotEnvViewer.hide') : $t('dotEnvViewer.reveal')}>
                  {revealed.has(i) ? '🙈' : '👁'}
                </button>
                <button type="button" class="dotenv-icon-btn" on:click={() => copyValue(i)}
                  title={$t('dotEnvViewer.copy')} aria-label={$t('dotEnvViewer.copy')}>
                  {copiedIndex === i ? '✓' : '⧉'}
                </button>
              </div>
            </div>
          {/each}
        </div>
        <p class="dotenv-footnote">{$t('dotEnvViewer.footnote', { count: entries.length })}</p>
      {/if}
    {/if}
  </div>
</div>

<style>
  .dotenv-modal {
    background: var(--bg-secondary, #1c1f26);
    border: 1px solid var(--border-color, #2c303a);
    border-radius: 10px;
    width: min(680px, 92vw);
    max-height: 84vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 18px 50px rgba(0, 0, 0, 0.5);
  }
  .dotenv-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 18px;
    border-bottom: 1px solid var(--border-color, #2c303a);
  }
  .dotenv-header h3 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }
  .dotenv-target {
    color: var(--text-secondary, #8a93a6);
    font-weight: 400;
  }
  .dotenv-consent,
  .dotenv-state,
  .dotenv-actions {
    padding: 16px 18px;
  }
  .dotenv-intro { margin: 0 0 10px; line-height: 1.5; }
  .dotenv-security {
    margin: 0;
    padding: 10px 12px;
    background: rgba(120, 160, 255, 0.08);
    border: 1px solid rgba(120, 160, 255, 0.25);
    border-radius: 6px;
    font-size: 13px;
    line-height: 1.5;
    color: var(--text-secondary, #aab3c5);
  }
  .dotenv-actions {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
  }
  .dotenv-error { color: var(--danger-color, #ff6b6b); white-space: pre-wrap; }
  .dotenv-subhead {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 18px;
    border-bottom: 1px solid var(--border-color, #2c303a);
  }
  .dotenv-source {
    font-size: 12px;
    color: var(--text-secondary, #8a93a6);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dotenv-bulk { display: flex; gap: 12px; flex-shrink: 0; }
  .dotenv-link {
    background: none;
    border: none;
    color: var(--accent-color, #6ea8ff);
    cursor: pointer;
    font-size: 12px;
    padding: 0;
  }
  .dotenv-link:hover { text-decoration: underline; }
  .dotenv-list {
    overflow-y: auto;
    padding: 6px 10px;
  }
  .dotenv-row {
    display: grid;
    grid-template-columns: minmax(120px, 0.4fr) 1fr auto;
    align-items: center;
    gap: 10px;
    padding: 7px 8px;
    border-radius: 6px;
  }
  .dotenv-row:hover { background: rgba(255, 255, 255, 0.03); }
  .dotenv-key {
    font-family: var(--mono-font, monospace);
    font-size: 13px;
    color: var(--accent-color, #8ab4ff);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .dotenv-value {
    font-family: var(--mono-font, monospace);
    font-size: 13px;
    color: var(--text-secondary, #9aa3b5);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    user-select: none;
  }
  .dotenv-value.revealed {
    color: var(--text-primary, #e6e9f0);
    user-select: text;
  }
  .dotenv-row-actions { display: flex; gap: 4px; }
  .dotenv-icon-btn {
    background: none;
    border: 1px solid transparent;
    border-radius: 5px;
    cursor: pointer;
    font-size: 14px;
    line-height: 1;
    padding: 4px 6px;
    color: var(--text-secondary, #9aa3b5);
  }
  .dotenv-icon-btn:hover {
    background: rgba(255, 255, 255, 0.06);
    border-color: var(--border-color, #2c303a);
  }
  .dotenv-footnote {
    margin: 0;
    padding: 10px 18px 16px;
    font-size: 11px;
    color: var(--text-secondary, #707888);
  }
</style>
