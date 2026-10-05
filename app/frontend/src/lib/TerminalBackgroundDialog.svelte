<script>
  import { createEventDispatcher, onDestroy, onMount } from 'svelte';
  import { t } from './i18n.js';
  import { modalFocus } from './actions/modalFocus.js';
  import { registerOverlay } from './actions/keyboardShortcuts.js';
  import { applyBackgroundToAll, applyTerminalBackground, deleteBackgroundImage, importBackgroundImage, listBackgroundImages } from './actions/backgroundActions.js';
  import { BACKGROUND_DEFAULTS, BACKGROUND_FITS, backgroundUrl } from './terminals/background.js';

  export let term; // the pane being edited (live store object)

  const dispatch = createEventDispatcher();
  let images = [];
  let error = '';
  let busy = false;
  // Edited live: every change is applied to the pane right away so the user
  // sees the result behind the dialog.
  let current = term?.background ? { ...term.background } : null;
  let opacity = Math.round((current?.opacity ?? BACKGROUND_DEFAULTS.opacity) * 100);
  let blur = current?.blur ?? BACKGROUND_DEFAULTS.blur;
  let fit = current?.fit ?? BACKGROUND_DEFAULTS.fit;

  const unregister = registerOverlay(() => true, close);
  onMount(refresh);
  onDestroy(unregister);

  async function refresh() {
    images = await listBackgroundImages();
  }

  function close() {
    dispatch('close');
  }

  function settings(id) {
    return { id, opacity: opacity / 100, blur, fit };
  }

  function pick(id) {
    error = '';
    current = id ? applyTerminalBackground(term.id, settings(id)) : applyTerminalBackground(term.id, null);
  }

  function reapply() {
    if (current) current = applyTerminalBackground(term.id, settings(current.id));
  }

  async function importImage() {
    if (busy) return;
    busy = true;
    error = '';
    try {
      const rec = await importBackgroundImage();
      if (rec?.id) {
        await refresh();
        pick(rec.id);
      }
    } catch (e) {
      error = e?.message || String(e);
    } finally {
      busy = false;
    }
  }

  async function remove(id) {
    error = '';
    try {
      await deleteBackgroundImage(id);
      if (current?.id === id) current = null;
      await refresh();
    } catch (e) {
      error = e?.message || String(e);
    }
  }

  function applyAll() {
    applyBackgroundToAll(current ? settings(current.id) : null);
  }

  function formatSize(bytes) {
    if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`;
    return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="modal-overlay bg-dialog-overlay" on:click|self={close}>
  <div class="bg-dialog" role="dialog" aria-modal="true" aria-labelledby="bg-dialog-title" use:modalFocus>
    <div class="bg-dialog-head">
      <h2 id="bg-dialog-title">{$t('background.title')} · <span class="bg-dialog-pane">{term?.name}</span></h2>
      <button type="button" class="bg-close" on:click={close} aria-label={$t('background.close')}>&times;</button>
    </div>
    <p class="bg-dialog-hint">{$t('background.hint')}</p>

    <div class="bg-grid" role="listbox" aria-label={$t('background.images')}>
      <button type="button" class="bg-tile bg-tile-none" class:selected={!current} role="option" aria-selected={!current} on:click={() => pick('')}>
        <span>{$t('background.none')}</span>
      </button>
      {#each images as img (img.id)}
        <div class="bg-tile-wrap">
          <button
            type="button"
            class="bg-tile"
            class:selected={current?.id === img.id}
            role="option"
            aria-selected={current?.id === img.id}
            style="background-image: url({backgroundUrl(img.id)})"
            title="{img.name} · {formatSize(img.size)}"
            on:click={() => pick(img.id)}
          >
            <span class="bg-tile-name">{img.name}</span>
          </button>
          <button type="button" class="bg-tile-delete" on:click|stopPropagation={() => remove(img.id)} title={$t('background.delete')} aria-label={$t('background.delete')}>&times;</button>
        </div>
      {/each}
      <button type="button" class="bg-tile bg-tile-import" on:click={importImage} disabled={busy}>
        <span class="bg-tile-plus">+</span>
        <span>{busy ? $t('background.importing') : $t('background.import')}</span>
      </button>
    </div>

    {#if error}
      <div class="bg-error" role="alert">{error}</div>
    {/if}

    <div class="bg-controls" class:disabled={!current}>
      <label class="bg-control">
        <span>{$t('background.opacity')}</span>
        <input type="range" min="5" max="100" step="5" bind:value={opacity} on:input={reapply} disabled={!current} />
        <output>{opacity}%</output>
      </label>
      <label class="bg-control">
        <span>{$t('background.blur')}</span>
        <input type="range" min="0" max="24" step="1" bind:value={blur} on:input={reapply} disabled={!current} />
        <output>{blur}px</output>
      </label>
      <label class="bg-control">
        <span>{$t('background.fit')}</span>
        <select bind:value={fit} on:change={reapply} disabled={!current}>
          {#each BACKGROUND_FITS as f}
            <option value={f}>{$t(`background.fit_${f}`)}</option>
          {/each}
        </select>
      </label>
    </div>

    <div class="bg-actions">
      <button type="button" class="bg-btn" on:click={applyAll} title={$t('background.applyAllTitle')}>{$t('background.applyAll')}</button>
      <button type="button" class="bg-btn bg-btn-primary" on:click={close}>{$t('background.done')}</button>
    </div>
  </div>
</div>

<style>
  .bg-dialog-overlay { z-index: 40; }
  .bg-dialog {
    width: min(560px, 100%);
    max-height: 88vh;
    overflow: auto;
    background: var(--bg-deep);
    border: 1px solid var(--border-accent);
    border-radius: var(--radius-lg);
    box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
    padding: 1rem 1.1rem 1.1rem;
    color: var(--text-primary);
    display: grid;
    gap: 0.8rem;
  }
  .bg-dialog-head { display: flex; align-items: center; justify-content: space-between; gap: 0.6rem; }
  .bg-dialog-head h2 { margin: 0; font-size: 0.98rem; font-weight: 600; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .bg-dialog-pane { color: var(--text-secondary); font-weight: 500; }
  .bg-close { background: transparent; border: none; color: var(--text-secondary); font-size: 1.3rem; line-height: 1; cursor: pointer; padding: 0 0.2rem; }
  .bg-close:hover { color: var(--text-primary); }
  .bg-dialog-hint { margin: 0; font-size: 0.8rem; color: var(--text-secondary); }

  .bg-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(112px, 1fr)); gap: 0.5rem; }
  .bg-tile-wrap { position: relative; }
  .bg-tile {
    width: 100%;
    aspect-ratio: 16 / 10;
    border-radius: var(--radius-md);
    border: 2px solid var(--border-dim);
    background: var(--bg-void) center / cover no-repeat;
    color: var(--text-secondary);
    font-size: 0.75rem;
    font-family: inherit;
    cursor: pointer;
    display: flex;
    align-items: flex-end;
    justify-content: center;
    padding: 0;
    overflow: hidden;
    transition: border-color 120ms ease;
  }
  .bg-tile:hover { border-color: var(--border-accent); }
  .bg-tile.selected { border-color: #63b3ed; box-shadow: 0 0 0 1px #63b3ed inset; }
  .bg-tile-none, .bg-tile-import { align-items: center; flex-direction: column; gap: 0.2rem; }
  .bg-tile-import { border-style: dashed; }
  .bg-tile-import:disabled { opacity: 0.6; cursor: progress; }
  .bg-tile-plus { font-size: 1.4rem; line-height: 1; color: var(--text-primary); }
  .bg-tile-name {
    width: 100%;
    padding: 0.2rem 0.35rem;
    background: rgba(12, 14, 20, 0.72);
    color: var(--text-primary);
    font-size: 0.68rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    text-align: left;
  }
  .bg-tile-delete {
    position: absolute;
    top: 4px;
    right: 4px;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    border: none;
    background: rgba(12, 14, 20, 0.8);
    color: var(--text-secondary);
    font-size: 0.9rem;
    line-height: 1;
    cursor: pointer;
    opacity: 0;
    transition: opacity 120ms ease;
  }
  .bg-tile-wrap:hover .bg-tile-delete, .bg-tile-delete:focus-visible { opacity: 1; }
  .bg-tile-delete:hover { color: #f87171; }

  .bg-error { font-size: 0.8rem; color: #f87171; }

  .bg-controls { display: grid; gap: 0.5rem; }
  .bg-controls.disabled { opacity: 0.55; }
  .bg-control { display: grid; grid-template-columns: 7.5rem 1fr 3.4rem; align-items: center; gap: 0.6rem; font-size: 0.8rem; color: var(--text-secondary); }
  .bg-control input[type='range'] { width: 100%; accent-color: #63b3ed; }
  .bg-control output { text-align: right; font-variant-numeric: tabular-nums; color: var(--text-primary); }
  .bg-control select {
    grid-column: 2 / 4;
    background: var(--bg-surface);
    color: var(--text-primary);
    border: 1px solid var(--border-dim);
    border-radius: var(--radius-sm, 4px);
    padding: 0.3rem 0.5rem;
    font: inherit;
  }

  .bg-actions { display: flex; justify-content: flex-end; gap: 0.5rem; }
  .bg-btn {
    background: var(--bg-surface);
    color: var(--text-primary);
    border: 1px solid var(--border-dim);
    border-radius: var(--radius-md);
    padding: 0.4rem 0.8rem;
    font: inherit;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .bg-btn:hover { border-color: var(--border-accent); }
  .bg-btn-primary { background: #2b4c6f; border-color: #3b6a99; }
  .bg-btn-primary:hover { background: #335a84; }
</style>
