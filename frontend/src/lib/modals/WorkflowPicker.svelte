<script>
  import { onMount } from 'svelte';
  import { t } from '../i18n.js';

  export let playbooks = [];
  export let onSelect = () => {};
  export let onClose = () => {};
  export let loading = false;

  let query = '';
  let inputEl;
  let activeIndex = 0;

  $: filtered = playbooks.filter((pb) => {
    const q = query.trim().toLowerCase();
    if (!q) return true;
    return [pb.name, pb.description, pb.mode].filter(Boolean).join(' ').toLowerCase().includes(q);
  });
  $: if (activeIndex > filtered.length - 1) activeIndex = Math.max(0, filtered.length - 1);

  onMount(() => inputEl?.focus());

  function choose(pb) {
    if (pb) onSelect(pb);
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
  <div class="workflow-picker" role="dialog" aria-modal="true">
    <input
      bind:this={inputEl}
      bind:value={query}
      on:keydown={onKeydown}
      class="workflow-search-input"
      placeholder={$t('workflowPicker.search')}
    />
    {#if loading}
      <div class="workflow-picker-loading">{$t('workflowPicker.loading')}</div>
    {:else}
      <ul class="workflow-picker-list">
        {#if filtered.length === 0}
          <li class="workflow-picker-empty">{$t('workflowPicker.empty')}</li>
        {:else}
          {#each filtered as pb, i (pb.id)}
            <li>
              <button
                type="button"
                class="workflow-picker-item"
                class:active={i === activeIndex}
                on:click={() => choose(pb)}
                on:mouseenter={() => (activeIndex = i)}
              >
                <span class="workflow-picker-top">
                  <strong>{pb.name}</strong>
                  <span class="workflow-picker-meta">
                    <span class="workflow-picker-mode">{pb.mode}</span>
                    <span class="workflow-picker-steps">{pb.steps?.length || 0} steps</span>
                  </span>
                </span>
                {#if pb.description}<small>{pb.description}</small>{/if}
              </button>
            </li>
          {/each}
        {/if}
      </ul>
    {/if}
    <div class="workflow-picker-hint">{$t('workflowPicker.hint')}</div>
  </div>
</div>

<style>
  .workflow-picker {
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

  .workflow-picker .workflow-search-input {
    margin: 0;
    border: none;
    border-bottom: 1px solid var(--border-subtle);
    border-radius: 0;
    padding: 0.85rem 1rem;
    font-size: 0.9rem;
    background: var(--bg-deep);
    color: var(--text-primary);
    font-family: var(--font-sans);
  }

  .workflow-picker .workflow-search-input:focus {
    outline: none;
  }

  .workflow-picker-list {
    list-style: none;
    margin: 0;
    padding: 0.35rem;
    overflow-y: auto;
  }

  .workflow-picker-loading,
  .workflow-picker-empty {
    padding: 1rem;
    text-align: center;
    color: var(--text-muted);
    font-size: 0.82rem;
  }

  .workflow-picker-item {
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

  .workflow-picker-item.active {
    background: var(--accent-glow);
    color: var(--text-primary);
  }

  .workflow-picker-top {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.6rem;
  }

  .workflow-picker-item strong {
    font-size: 0.85rem;
    color: var(--text-primary);
  }

  .workflow-picker-meta {
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
  }

  .workflow-picker-mode {
    font-size: 0.68rem;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .workflow-picker-steps {
    font-size: 0.68rem;
    color: var(--text-muted);
  }

  .workflow-picker-item small {
    font-size: 0.74rem;
    color: var(--text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .workflow-picker-hint {
    border-top: 1px solid var(--border-subtle);
    padding: 0.5rem 1rem;
    color: var(--text-muted);
    font-size: 0.7rem;
  }
</style>
