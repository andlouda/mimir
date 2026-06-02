<script>
  // AI hub landing view (presentational). Reads settings/config and forwards
  // navigation actions. Strings via i18n; styles from the global stylesheets.
  import { t } from '../i18n.js';

  export let settings;                 // aiSettings
  export let flow;                     // aiToolFlowConfig
  export let promptIntroPreview = '';
  export let usingDefaultIntro = false;
  export let onOpenCatalog = () => {};
  export let onOpenSettings = () => {};
  export let onOpenLogs = () => {};

  function getFilterPills(values, fallback) {
    if (!Array.isArray(values) || values.length === 0) {
      return [fallback];
    }
    return values;
  }

  $: providerLabel = settings.provider === 'ollama' ? 'Ollama' : 'OpenAI';
</script>

<div class="ai-hub">
  <div class="ai-hub-header">
    <div>
      <h2>{$t('aiHub.title')}</h2>
      <p>{$t('aiHub.subtitle')}</p>
    </div>
    <div class="ai-hub-badges">
      <span class="ai-hub-badge">{$t('aiHub.providerBadge', { provider: providerLabel })}</span>
      <span class="ai-hub-badge">{$t('aiHub.modeBadge', { mode: flow.execution.workflowMode })}</span>
    </div>
  </div>

  <div class="ai-hub-grid">
    <button type="button" class="ai-hub-card" on:click={onOpenCatalog}>
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x2692;</span>
        <span class="ai-hub-link">{$t('aiHub.cards.catalog.action')}</span>
      </div>
      <strong>{$t('aiHub.cards.catalog.title')}</strong>
      <p>{$t('aiHub.cards.catalog.desc')}</p>
    </button>

    <button type="button" class="ai-hub-card" on:click={onOpenSettings}>
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#9881;</span>
        <span class="ai-hub-link">{$t('aiHub.cards.settings.action')}</span>
      </div>
      <strong>{$t('aiHub.cards.settings.title')}</strong>
      <p>{$t('aiHub.cards.settings.desc')}</p>
    </button>

    <button type="button" class="ai-hub-card" on:click={onOpenLogs}>
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x1F4DC;</span>
        <span class="ai-hub-link">{$t('aiHub.cards.logs.action')}</span>
      </div>
      <strong>{$t('aiHub.cards.logs.title')}</strong>
      <p>{$t('aiHub.cards.logs.desc')}</p>
    </button>
  </div>

  <div class="ai-hub-summary">
    <button type="button" class="ai-summary-card ai-summary-card-action" on:click={onOpenSettings}>
      <span class="ai-summary-label">{$t('aiHub.summary.promptIntro')}</span>
      <pre>{promptIntroPreview}</pre>
      {#if usingDefaultIntro}
        <span class="ai-summary-note">{$t('aiHub.summary.defaultIntroNote')}</span>
      {/if}
      <span class="ai-summary-link">{$t('aiHub.summary.openSettings')}</span>
    </button>
    <button
      type="button"
      class="ai-summary-card ai-summary-card-action"
      on:click={onOpenSettings}
    >
      <span class="ai-summary-label">{$t('aiHub.summary.toolFilter')}</span>
      <div class="filter-group">
        <span class="filter-group-label">{$t('aiHub.summary.includeCategories')}</span>
        <div class="filter-pill-row">
          {#each getFilterPills(flow.toolFilter.includeCategories, $t('aiHub.summary.all')) as value (`inc-cat-${value}`)}
            <span class="filter-pill">{value}</span>
          {/each}
        </div>
      </div>
      <div class="filter-group">
        <span class="filter-group-label">{$t('aiHub.summary.excludeCategories')}</span>
        <div class="filter-pill-row">
          {#each getFilterPills(flow.toolFilter.excludeCategories, $t('aiHub.summary.none')) as value (`exc-cat-${value}`)}
            <span class="filter-pill filter-pill-muted">{value}</span>
          {/each}
        </div>
      </div>
      <div class="filter-group">
        <span class="filter-group-label">{$t('aiHub.summary.includeToolIds')}</span>
        <div class="filter-pill-row">
          {#each getFilterPills(flow.toolFilter.includeToolIds, $t('aiHub.summary.all')) as value (`inc-id-${value}`)}
            <span class="filter-pill">{value}</span>
          {/each}
        </div>
      </div>
      <div class="filter-group">
        <span class="filter-group-label">{$t('aiHub.summary.excludeToolIds')}</span>
        <div class="filter-pill-row">
          {#each getFilterPills(flow.toolFilter.excludeToolIds, $t('aiHub.summary.none')) as value (`exc-id-${value}`)}
            <span class="filter-pill filter-pill-muted">{value}</span>
          {/each}
        </div>
      </div>
      <span class="ai-summary-link">{$t('aiHub.summary.editFilters')}</span>
    </button>
    <button type="button" class="ai-summary-card ai-summary-card-action ai-summary-card-wide" on:click={onOpenSettings}>
      <span class="ai-summary-label">{$t('aiHub.summary.modelRuntime')}</span>
      <div class="runtime-grid">
        <div class="runtime-item">
          <span class="runtime-label">{$t('aiHub.summary.provider')}</span>
          <span class="runtime-value">{providerLabel}</span>
        </div>
        <div class="runtime-item">
          <span class="runtime-label">{$t('aiHub.summary.model')}</span>
          <span class="runtime-value">{settings.model || $t('aiHub.summary.notSet')}</span>
        </div>
        <div class="runtime-item">
          <span class="runtime-label">{$t('aiHub.summary.endpoint')}</span>
          <span class="runtime-value">{settings.baseUrl || $t('aiHub.summary.defaultEndpoint')}</span>
        </div>
        <div class="runtime-item">
          <span class="runtime-label">{$t('aiHub.summary.workflowMode')}</span>
          <span class="runtime-value">{flow.execution.workflowMode}</span>
        </div>
        <div class="runtime-item">
          <span class="runtime-label">{$t('aiHub.summary.terminalContext')}</span>
          <span class="runtime-value">{flow.prompt.includeTerminalOutput ? $t('aiHub.summary.contextChars', { n: flow.prompt.maxTerminalContext }) : $t('aiHub.summary.disabled')}</span>
        </div>
        <div class="runtime-item">
          <span class="runtime-label">{$t('aiHub.summary.lowRiskApproval')}</span>
          <span class="runtime-value">{flow.approval.requireApprovalForLow ? $t('aiHub.summary.required') : $t('aiHub.summary.notRequired')}</span>
        </div>
      </div>
      <span class="ai-summary-link">{$t('aiHub.summary.adjustRuntime')}</span>
    </button>
  </div>
</div>
