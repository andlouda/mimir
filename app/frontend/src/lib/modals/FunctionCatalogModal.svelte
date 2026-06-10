<script>
  // Function catalog browser. Owns its own UI state and calls the backend
  // (ExplainFunction / discoveryApi) directly; cross-cutting concerns are
  // emitted to the parent. Styles come from the global stylesheets (styles/).
  import { t } from '../i18n.js';
  import { runDiscovery } from '../templates/discoveryApi.js';

  export let catalog = [];                 // function catalog entries (loaded by parent)
  export let discoveryTerminalType = '';   // computed by parent for discovery runs
  export let discoveryTerminalId = null;   // active terminal, when one exists
  export let onClose = () => {};
  export let onAddToWorkflow = () => {};   // (entries) => parent queues + navigates
  export let onError = () => {};           // (message) => parent surfaces error

  let query = '';
  let selectedFunction = null;
  let selectedFunctionIds = [];
  let functionQuestion = '';
  let functionExplanation = '';
  let functionExplanationLoading = false;
  let discoveryInputValues = {};
  let discoveryResults = [];
  let discoveryLoading = false;

  $: filtered = catalog.filter((entry) => {
    const q = query.trim().toLowerCase();
    if (!q) return true;
    const text = [
      entry.name,
      entry.kind,
      entry.category,
      entry.description,
      ...(entry.parameters || []).map((param) => param.name),
    ].join(' ').toLowerCase();
    return text.includes(q);
  });

  function toggleFunctionSelection(entry) {
    selectedFunction = entry;
    functionExplanation = '';
    discoveryResults = [];
    discoveryLoading = false;

    const nextInputs = {};
    for (const parameter of (entry.parameters || [])) {
      nextInputs[parameter.name] = discoveryInputValues[parameter.name] || '';
    }
    discoveryInputValues = nextInputs;

    if (selectedFunctionIds.includes(entry.id)) {
      selectedFunctionIds = selectedFunctionIds.filter((id) => id !== entry.id);
    } else {
      selectedFunctionIds = [...selectedFunctionIds, entry.id];
    }
  }

  function addToWorkflow() {
    const discoveryEntries = selectedFunctionIds.length > 0
      ? filtered.filter((entry) => selectedFunctionIds.includes(entry.id) && entry.kind === 'discovery_tool')
      : (selectedFunction && selectedFunction.kind === 'discovery_tool' ? [selectedFunction] : []);
    if (discoveryEntries.length > 0) {
      onError('Discovery functions are preview tools right now and cannot yet be added as workflow steps.');
      return;
    }

    const sourceEntries = selectedFunctionIds.length > 0
      ? filtered.filter((entry) => selectedFunctionIds.includes(entry.id))
      : (selectedFunction ? [selectedFunction] : []);

    const entries = sourceEntries.map((entry, index) => ({
      ...entry,
      _seedId: `${entry.id}-${Date.now()}-${index}-${Math.random().toString(36).slice(2, 7)}`
    }));

    if (entries.length === 0) {
      onError('Select at least one function first.');
      return;
    }
    onAddToWorkflow(entries);
  }

  async function explainSelectedFunction() {
    if (!selectedFunction) {
      onError('Please select a function first.');
      return;
    }
    functionExplanationLoading = true;
    functionExplanation = '';
    try {
      functionExplanation = await window['go']['main']['App']['ExplainFunction'](
        selectedFunction.id,
        functionQuestion
      );
    } catch (error) {
      onError(`Function explanation failed: ${error.message || error}`);
    } finally {
      functionExplanationLoading = false;
    }
  }

  async function runSelectedDiscovery() {
    if (!selectedFunction || selectedFunction.kind !== 'discovery_tool' || !selectedFunction.discoveryTool) {
      onError('No discovery function selected.');
      return;
    }
    discoveryLoading = true;
    discoveryResults = [];
    try {
      const payload = await runDiscovery(
        discoveryTerminalId,
        selectedFunction.discoveryTool,
        discoveryTerminalType,
        JSON.stringify(discoveryInputValues || {})
      );
      discoveryResults = JSON.parse(payload);
    } catch (error) {
      onError(`Discovery failed: ${error.message || error}`);
      discoveryResults = [];
    } finally {
      discoveryLoading = false;
    }
  }
</script>

<div class="modal-overlay" on:click={onClose} on:keydown={(e) => { if (e.key === 'Escape') onClose(); }} tabindex="0" role="button">
  <div class="function-catalog-modal" role="dialog" aria-modal="true" tabindex="-1" on:click|stopPropagation on:keydown|stopPropagation>
    <div class="template-prompt-header">
      <h3>{$t('functionCatalog.title')}</h3>
      <button type="button" class="modal-close-button" on:click={onClose}>&#x2715;</button>
    </div>
    <p class="template-prompt-text">{$t('functionCatalog.subtitle')}</p>
    <input
      type="text"
      bind:value={query}
      class="template-search-input"
      placeholder={$t('functionCatalog.search')}
    />
    <div class="function-catalog-layout">
      <div class="function-catalog-list">
        {#each filtered as entry (entry.id)}
          <button
            type="button"
            class:selected-function={selectedFunction && selectedFunction.id === entry.id}
            class:selected-function-multi={selectedFunctionIds.includes(entry.id)}
            class="function-catalog-item"
            on:click={() => toggleFunctionSelection(entry)}
          >
            <div class="function-catalog-item-top">
              <span class="function-select-toggle">
                {selectedFunctionIds.includes(entry.id) ? $t('functionCatalog.selected') : $t('functionCatalog.clickToSelect')}{selectedFunction && selectedFunction.id === entry.id ? $t('functionCatalog.active') : ''}
              </span>
              <span class="function-catalog-open">{$t('functionCatalog.viewDetails')}</span>
            </div>
            {#if selectedFunctionIds.includes(entry.id)}
              <span class="function-selection-index">{selectedFunctionIds.indexOf(entry.id) + 1}</span>
            {/if}
            <strong>{entry.name}</strong>
            <span>{entry.category} · {entry.kind}</span>
            <small>{entry.description}</small>
          </button>
        {/each}
      </div>
      <div class="function-catalog-detail">
        {#if selectedFunction}
          <div class="function-detail-head">
            <h4>{selectedFunction.name}</h4>
            <span>{selectedFunction.category} · {selectedFunction.kind}</span>
          </div>
          <p>{selectedFunction.description}</p>
          {#if selectedFunction.parameters && selectedFunction.parameters.length > 0}
            <div class="function-detail-section">
              <span class="ai-result-label">{$t('functionCatalog.parameters')}</span>
              <ul class="function-parameter-list">
                {#each selectedFunction.parameters as parameter (`${selectedFunction.id}-${parameter.name}`)}
                  <li>{parameter.name}{parameter.required ? $t('functionCatalog.required') : ''}</li>
                {/each}
              </ul>
            </div>
          {/if}
          {#if selectedFunction.kind === 'discovery_tool'}
            <div class="function-detail-section">
              <span class="ai-result-label">{$t('functionCatalog.discoveryPreview')}</span>
              <p class="template-prompt-text">{$t('functionCatalog.discoveryHint')}</p>
              {#if selectedFunction.parameters && selectedFunction.parameters.length > 0}
                <div class="step-input-grid">
                  {#each selectedFunction.parameters as parameter (`${selectedFunction.id}-discovery-${parameter.name}`)}
                    <label class="form-field">
                      <span>{parameter.name}</span>
                      <input type="text" bind:value={discoveryInputValues[parameter.name]} placeholder={parameter.name} />
                    </label>
                  {/each}
                </div>
              {/if}
              <div class="function-discovery-meta">
                <span>{$t('functionCatalog.terminalType', { type: discoveryTerminalType })}</span>
                <span>{$t('functionCatalog.toolId', { id: selectedFunction.discoveryTool })}</span>
              </div>
              {#if discoveryResults.length > 0}
                <div class="ai-result-block">
                  <span class="ai-result-label">{$t('functionCatalog.discoveryResults')}</span>
                  <pre>{discoveryResults.join('\n')}</pre>
                </div>
              {/if}
            </div>
          {/if}
          <label class="template-prompt-field">
            <span>{$t('functionCatalog.askAboutFn')}</span>
            <textarea bind:value={functionQuestion} placeholder={$t('functionCatalog.askPlaceholder')}></textarea>
          </label>
          {#if functionExplanation}
            <div class="ai-result-block">
              <span class="ai-result-label">{$t('functionCatalog.aiExplanation')}</span>
              <pre>{functionExplanation}</pre>
            </div>
          {/if}
          <div class="template-prompt-actions">
            {#if selectedFunction.kind === 'discovery_tool'}
              <button type="button" class="modal-secondary-button" on:click={runSelectedDiscovery} disabled={discoveryLoading}>
                {discoveryLoading ? $t('functionCatalog.runningDiscovery') : $t('functionCatalog.runDiscovery')}
              </button>
            {:else}
              <button type="button" class="modal-secondary-button" on:click={addToWorkflow}>
                {$t('functionCatalog.addToWorkflow')}{selectedFunctionIds.length > 0 ? ` (${selectedFunctionIds.length})` : ''}
              </button>
            {/if}
            <button type="button" class="modal-secondary-button" on:click={onClose}>{$t('functionCatalog.close')}</button>
            <button type="button" class="modal-primary-button" on:click={explainSelectedFunction} disabled={functionExplanationLoading}>
              {functionExplanationLoading ? $t('functionCatalog.running') : $t('functionCatalog.askAI')}
            </button>
          </div>
        {:else}
          <p class="template-prompt-text">{$t('functionCatalog.emptyDetail')}</p>
        {/if}
      </div>
    </div>
  </div>
</div>
