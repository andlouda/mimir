<script>
  // Presentational modal that collects variable values before running a
  // template. Logic (open/close/submit) stays in the parent; shared modal
  // styles come from the global ./modal.css (imported by App.svelte).
  import { t } from '../i18n.js';

  export let state;             // templatePromptState: { templateName, fields: [{ name, label, value }] }
  export let onClose = () => {};
  export let onSubmit = () => {};
  export let onFieldChange = () => {};
</script>

<div
  class="modal-overlay"
  on:click={onClose}
  on:keydown={(e) => { if (e.key === 'Escape') onClose(); }}
  tabindex="0"
  role="button"
>
  <div
    class="template-prompt-modal"
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    on:click|stopPropagation
    on:keydown|stopPropagation
  >
    <div class="template-prompt-header">
      <h3>{state.templateName}</h3>
      <button type="button" class="modal-close-button" on:click={onClose}>&#x2715;</button>
    </div>
    <p class="template-prompt-text">{$t('templatePrompt.intro')}</p>
    <div class="template-prompt-fields">
      {#each state.fields as field, index (field.name)}
        <label class="template-prompt-field">
          <span>{field.label}{#if field.loadingSuggestions} <span class="discovery-loading">…</span>{/if}</span>
          {#if field.discoveryTool}
            <select
              bind:value={state.fields[index].value}
              on:change={() => onFieldChange(state.fields[index])}
              on:keydown={(e) => { if (e.key === 'Enter') onSubmit(); }}
            >
              <option value="">{field.loadingSuggestions ? 'Loading...' : `${field.label}...`}</option>
              {#if field.value && !(field.suggestions || []).includes(field.value)}
                <option value={field.value}>{field.value}</option>
              {/if}
              {#each field.suggestions as s}
                <option value={s}>{s}</option>
              {/each}
            </select>
            {#if field.suggestionError}
              <small class="discovery-error">{field.suggestionError}</small>
            {/if}
          {:else}
            <input
              type="text"
              bind:value={state.fields[index].value}
              placeholder={field.name}
              on:change={() => onFieldChange(state.fields[index])}
              on:keydown={(e) => { if (e.key === 'Enter') onSubmit(); }}
            />
          {/if}
        </label>
      {/each}
    </div>
    <div class="template-prompt-actions">
      <button type="button" class="modal-secondary-button" on:click={onClose}>{$t('templatePrompt.cancel')}</button>
      <button type="button" class="modal-primary-button" on:click={onSubmit}>{$t('templatePrompt.run')}</button>
    </div>
  </div>
</div>

<style>
  .discovery-loading {
    font-size: 0.75rem;
    opacity: 0.6;
  }
  .discovery-error {
    display: block;
    margin-top: 0.35rem;
    color: var(--warning);
    font-size: 0.72rem;
    line-height: 1.35;
    overflow-wrap: anywhere;
  }
  select {
    width: 100%;
    padding: 0.5rem;
    background: var(--bg-surface);
    color: var(--text-primary);
    border: 1px solid var(--border-dim);
    border-radius: var(--radius-sm);
    font-family: var(--font-sans);
    font-size: 0.85rem;
  }
</style>
