<script>
  // Command-palette-style template picker (opened via keyboard shortcut).
  // Search + keyboard-navigate templates; selecting one applies it via the
  // parent's onSelect (which runs the apply flow, prompting for variables).
  import { onMount } from 'svelte';
  import { t } from '../i18n.js';

  export let templates = [];
  export let onSelect = () => {};
  export let onClose = () => {};

  let query = '';
  let inputEl;
  let activeIndex = 0;

  $: filtered = templates.filter((tpl) => {
    const q = query.trim().toLowerCase();
    if (!q) return true;
    return [tpl.name, tpl.category, tpl.description].filter(Boolean).join(' ').toLowerCase().includes(q);
  });
  // Keep the active index within bounds as the list narrows.
  $: if (activeIndex > filtered.length - 1) activeIndex = Math.max(0, filtered.length - 1);

  onMount(() => inputEl?.focus());

  function choose(tpl) {
    if (tpl) onSelect(tpl.name);
  }

  function onKeydown(e) {
    if (e.key === 'Escape') {
      onClose();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      activeIndex = Math.min(activeIndex + 1, filtered.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      activeIndex = Math.max(activeIndex - 1, 0);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      choose(filtered[activeIndex]);
    }
  }
</script>

<div class="modal-overlay" on:click|self={onClose} on:keydown={(e) => { if (e.key === 'Escape') onClose(); }} role="presentation">
  <div class="template-picker" role="dialog" aria-modal="true">
    <input
      bind:this={inputEl}
      bind:value={query}
      on:keydown={onKeydown}
      class="template-search-input"
      placeholder={$t('templatePicker.search')}
    />
    <ul class="template-picker-list">
      {#if filtered.length === 0}
        <li class="template-picker-empty">{$t('templatePicker.empty')}</li>
      {:else}
        {#each filtered as tpl, i (tpl.name)}
          <li>
            <button
              type="button"
              class="template-picker-item"
              class:active={i === activeIndex}
              on:click={() => choose(tpl)}
              on:mouseenter={() => (activeIndex = i)}
            >
              <span class="template-picker-top">
                <strong>{tpl.name}</strong>
                <span class="template-picker-category">{tpl.category}</span>
              </span>
              {#if tpl.description}<small>{tpl.description}</small>{/if}
            </button>
          </li>
        {/each}
      {/if}
    </ul>
    <div class="template-picker-hint">{$t('templatePicker.hint')}</div>
  </div>
</div>

<style>
  .template-picker {
    width: min(560px, 100%);
    max-height: 70vh;
    margin-top: 10vh;
    align-self: flex-start;
    display: flex;
    flex-direction: column;
    background: var(--bg-deep);
    border: 1px solid var(--border-accent);
    border-radius: var(--radius-lg);
    box-shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
    overflow: hidden;
  }

  .template-picker .template-search-input {
    margin: 0;
    border: none;
    border-bottom: 1px solid var(--border-subtle);
    border-radius: 0;
    padding: 0.85rem 1rem;
    font-size: 0.9rem;
  }

  .template-picker-list {
    list-style: none;
    margin: 0;
    padding: 0.35rem;
    overflow-y: auto;
  }

  .template-picker-empty {
    padding: 1rem;
    text-align: center;
    color: var(--text-muted);
    font-size: 0.82rem;
  }

  .template-picker-item {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    width: 100%;
    text-align: left;
    background: none;
    border: none;
    border-radius: var(--radius-sm);
    padding: 0.5rem 0.7rem;
    color: var(--text-secondary);
    font-family: var(--font-sans);
    cursor: pointer;
  }

  .template-picker-item.active {
    background: var(--accent-glow);
    color: var(--text-primary);
  }

  .template-picker-top {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.6rem;
  }

  .template-picker-item strong {
    font-size: 0.85rem;
    color: var(--text-primary);
  }

  .template-picker-category {
    font-size: 0.68rem;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .template-picker-item small {
    font-size: 0.74rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .template-picker-hint {
    border-top: 1px solid var(--border-subtle);
    padding: 0.5rem 1rem;
    color: var(--text-muted);
    font-size: 0.7rem;
  }
</style>
