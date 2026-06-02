<script>
  // Presentational modal that collects variable values before running a
  // template. Logic (open/close/submit) stays in the parent; shared modal
  // styles come from the global ./modal.css (imported by App.svelte).
  import { t } from '../i18n.js';

  export let state;             // templatePromptState: { templateName, fields: [{ name, label, value }] }
  export let onClose = () => {};
  export let onSubmit = () => {};
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
          <span>{field.label}</span>
          <input
            type="text"
            bind:value={state.fields[index].value}
            placeholder={field.name}
            on:keydown={(e) => { if (e.key === 'Enter') onSubmit(); }}
          />
        </label>
      {/each}
    </div>
    <div class="template-prompt-actions">
      <button type="button" class="modal-secondary-button" on:click={onClose}>{$t('templatePrompt.cancel')}</button>
      <button type="button" class="modal-primary-button" on:click={onSubmit}>{$t('templatePrompt.run')}</button>
    </div>
  </div>
</div>
