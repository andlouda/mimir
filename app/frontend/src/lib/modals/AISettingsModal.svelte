<script>
  // Presentational AI Settings modal. Logic stays in the parent; the three
  // config objects are two-way bound, actions are forwarded via callbacks.
  // All styles (shared modal + AI-settings classes) come from the global CSS.
  import { t } from '../i18n.js';

  export let settings;             // aiSettings (bind)
  export let flow;                 // aiToolFlowConfig (bind)
  export let lists;                // aiToolFlowLists (bind)
  export let providers = [];       // AIProviderDescriptor[] from the backend registry
  export let promptPreview = '';   // derived: getAIToolPromptPreview()
  export let onClose = () => {};
  export let onSave = () => {};
  export let onProviderChange = () => {};
  export let onUseDevOpsExample = () => {};

  $: currentProvider = providers.find((p) => p.id === settings.provider) || null;
  $: showApiKey = !currentProvider || currentProvider.supportsApiKey !== false;
  $: apiKeyRequired = !!(currentProvider && currentProvider.requiresApiKey);
  $: modelPlaceholder = (currentProvider && currentProvider.defaultModel) || 'model name';
  $: urlPlaceholder = (currentProvider && currentProvider.defaultBaseUrl) || 'https://...';
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
      <h3>{$t('aiSettings.title')}</h3>
      <button type="button" class="modal-close-button" on:click={onClose}>&#x2715;</button>
    </div>
    <p class="template-prompt-text">{$t('aiSettings.subtitle')}</p>
    <div class="template-prompt-fields">
      <div class="ai-settings-section">
        <h4>{$t('aiSettings.sections.provider')}</h4>
      </div>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.provider')}</span>
        <select bind:value={settings.provider} on:change={(e) => onProviderChange(e.target.value)}>
          {#each providers as p (p.id)}
            <option value={p.id}>{p.label}</option>
          {/each}
        </select>
      </label>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.model')}</span>
        <input type="text" bind:value={settings.model} placeholder={modelPlaceholder} />
      </label>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.apiUrl')}</span>
        <input type="text" bind:value={settings.baseUrl} placeholder={urlPlaceholder} />
      </label>
      {#if showApiKey}
        <label class="template-prompt-field">
          <span>{$t('aiSettings.apiKey')}{#if !apiKeyRequired} {$t('aiSettings.optional')}{/if}</span>
          <input type="password" bind:value={settings.apiKey} placeholder={apiKeyRequired ? $t('aiSettings.apiKeyRequired') : $t('aiSettings.apiKeyOptional')} />
        </label>
      {/if}
      <div class="ai-settings-section">
        <h4>{$t('aiSettings.sections.prompt')}</h4>
      </div>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.prePrompt')}</span>
        <textarea
          bind:value={flow.prompt.prePrompt}
          placeholder={$t('aiSettings.prePromptPlaceholder')}
        ></textarea>
      </label>
      <div class="inline-action-row">
        <button type="button" class="modal-secondary-button" on:click={onUseDevOpsExample}>{$t('aiSettings.useDevOpsExample')}</button>
      </div>
      <p class="template-prompt-help">{$t('aiSettings.prePromptHelp')}</p>
      <details class="guardrail-preview">
        <summary>{$t('aiSettings.previewSummary')}</summary>
        <pre>{promptPreview}</pre>
      </details>
      <div class="ai-settings-grid">
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.prompt.requireStableToolId} disabled />
          <span>{$t('aiSettings.checkboxes.requireStableToolId')}</span>
        </label>
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.prompt.includeTerminalOutput} />
          <span>{$t('aiSettings.checkboxes.includeTerminalOutput')}</span>
        </label>
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.prompt.includeRisk} />
          <span>{$t('aiSettings.checkboxes.includeRisk')}</span>
        </label>
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.prompt.includeCategory} />
          <span>{$t('aiSettings.checkboxes.includeCategory')}</span>
        </label>
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.prompt.allowTemplateNameFallback} disabled />
          <span>{$t('aiSettings.checkboxes.allowTemplateNameFallback')}</span>
        </label>
      </div>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.maxContext')}</span>
        <input type="number" min="0" step="100" bind:value={flow.prompt.maxTerminalContext} />
      </label>
      <div class="ai-settings-section">
        <h4>{$t('aiSettings.sections.toolFilter')}</h4>
      </div>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.includeCategories')}</span>
        <input type="text" bind:value={lists.includeCategories} placeholder="Network, Kubernetes, AI" />
      </label>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.excludeCategories')}</span>
        <input type="text" bind:value={lists.excludeCategories} placeholder="Storage, Cleanup" />
      </label>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.includeToolIds')}</span>
        <input type="text" bind:value={lists.includeToolIds} placeholder="template:Ping Google, template:DNS Lookup" />
      </label>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.excludeToolIds')}</span>
        <input type="text" bind:value={lists.excludeToolIds} placeholder="template:Clean Temp Files" />
      </label>
      <div class="ai-settings-section">
        <h4>{$t('aiSettings.sections.approval')}</h4>
      </div>
      <div class="ai-settings-grid">
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.approval.respectStepFlag} disabled />
          <span>{$t('aiSettings.checkboxes.respectStepFlag')}</span>
        </label>
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.approval.requireApprovalForLow} />
          <span>{$t('aiSettings.checkboxes.approvalLow')}</span>
        </label>
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.approval.requireApprovalForMedium} disabled />
          <span>{$t('aiSettings.checkboxes.approvalMedium')}</span>
        </label>
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.approval.requireApprovalForHigh} disabled />
          <span>{$t('aiSettings.checkboxes.approvalHigh')}</span>
        </label>
      </div>
      <div class="ai-settings-section">
        <h4>{$t('aiSettings.sections.execution')}</h4>
      </div>
      <div class="ai-settings-grid">
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.execution.enabled} />
          <span>{$t('aiSettings.checkboxes.executionEnabled')}</span>
        </label>
        <label class="template-checkbox-field">
          <input type="checkbox" bind:checked={flow.execution.forceRequiresApproval} />
          <span>{$t('aiSettings.checkboxes.alwaysApproval')}</span>
        </label>
      </div>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.workflowMode')} <small class="experimental-label">{$t('aiSettings.experimental')}</small></span>
        <select bind:value={flow.execution.workflowMode}>
          <option value="assist">{$t('aiSettings.workflowModes.assist')}</option>
          <option value="approve">{$t('aiSettings.workflowModes.approve')}</option>
          <option value="auto">{$t('aiSettings.workflowModes.auto')}</option>
        </select>
        <span class="field-hint">{$t('aiSettings.workflowModeHint')}</span>
      </label>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.workflowIdPrefix')}</span>
        <input type="text" bind:value={flow.execution.workflowIdPrefix} placeholder="ai-tool-run" />
      </label>
      <label class="template-prompt-field">
        <span>{$t('aiSettings.workflowName')}</span>
        <input type="text" bind:value={flow.execution.workflowName} placeholder="AI Tool Run" />
      </label>
    </div>
    <div class="template-prompt-actions">
      <button type="button" class="modal-secondary-button" on:click={onClose}>{$t('aiSettings.cancel')}</button>
      <button type="button" class="modal-primary-button" on:click={onSave}>{$t('aiSettings.save')}</button>
    </div>
  </div>
</div>
