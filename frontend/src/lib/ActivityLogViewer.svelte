<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { t } from './i18n.js';

  const dispatch = createEventDispatcher();

  const fallbackKinds = [
    'ai_interactions',
    'workflow_runs',
    'tool_executions',
    'security_events',
    'approval_events',
  ];

  let errorMessage = '';
  let loading = false;
  let kinds = [];
  let selectedKind = 'all';
  let limit = 200;
  let logs = [];
  let searchQuery = '';
  let selectedEntry = null;
  let copyMessage = '';

  function kindLabel(kind) {
    switch (kind) {
      case 'ai_interactions':
        return 'AI';
      case 'workflow_runs':
        return 'Workflows';
      case 'tool_executions':
        return 'Tools';
      case 'security_events':
        return 'Security';
      case 'approval_events':
        return 'Approvals';
      case 'all':
        return 'All';
      default:
        return kind.replaceAll('_', ' ');
    }
  }

  function formatTimestamp(timestamp) {
    if (!timestamp) {
      return 'Unknown time';
    }
    const parsed = new Date(timestamp);
    if (Number.isNaN(parsed.getTime())) {
      return timestamp;
    }
    return parsed.toLocaleString();
  }

  function entrySearchText(entry) {
    return [
      entry.kind,
      entry.title,
      entry.summary,
      JSON.stringify(entry.raw || {}),
    ].join(' ').toLowerCase();
  }

  function selectEntry(entry) {
    selectedEntry = entry;
    copyMessage = '';
  }

  function compactJSON(value) {
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }

  async function copySelectedJSON() {
    if (!selectedEntry) {
      return;
    }
    try {
      await navigator.clipboard.writeText(compactJSON(selectedEntry.raw));
      copyMessage = 'JSON copied.';
    } catch (error) {
      copyMessage = `Copy failed: ${error.message || error}`;
    }
  }

  function metadataEntries(metadata) {
    if (!metadata || typeof metadata !== 'object') {
      return [];
    }
    return Object.entries(metadata);
  }

  function detailPairs(entry) {
    if (!entry?.raw) {
      return [];
    }
    const raw = entry.raw;
    const keys = [
      ['workflowId', 'Workflow'],
      ['stepId', 'Step'],
      ['toolName', 'Tool'],
      ['toolId', 'Tool ID'],
      ['source', 'Source'],
      ['risk', 'Risk'],
      ['provider', 'Provider'],
      ['model', 'Model'],
      ['baseUrl', 'Endpoint'],
      ['mode', 'Mode'],
      ['terminalType', 'Terminal'],
      ['terminalName', 'Terminal Name'],
      ['event', 'Event'],
      ['operation', 'Operation'],
      ['path', 'Path'],
      ['reason', 'Reason'],
      ['error', 'Error'],
    ];

    return keys
      .map(([key, label]) => [label, raw[key]])
      .filter(([, value]) => value !== undefined && value !== null && String(value).trim() !== '');
  }

  function chipValues(value) {
    if (Array.isArray(value)) {
      return value;
    }
    return [];
  }

  async function loadKinds() {
    try {
      const payload = await window['go']['main']['App']['GetActivityLogKindsJSON']();
      const parsed = JSON.parse(payload);
      kinds = Array.isArray(parsed) && parsed.length > 0 ? parsed : fallbackKinds;
    } catch (error) {
      kinds = fallbackKinds;
      errorMessage = `Failed to load log kinds: ${error.message || error}`;
    }
  }

  async function loadLogs() {
    loading = true;
    try {
      const payload = await window['go']['main']['App']['GetActivityLogsJSON'](selectedKind, Number(limit) || 200);
      logs = JSON.parse(payload);
      if (!selectedEntry || !logs.find((entry) => entry.timestamp === selectedEntry.timestamp && entry.kind === selectedEntry.kind && entry.title === selectedEntry.title)) {
        selectedEntry = logs[0] || null;
      }
      errorMessage = '';
    } catch (error) {
      logs = [];
      selectedEntry = null;
      errorMessage = `Failed to load activity logs: ${error.message || error}`;
    } finally {
      loading = false;
    }
  }

  $: visibleLogs = logs.filter((entry) => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) {
      return true;
    }
    return entrySearchText(entry).includes(query);
  });

  $: selectedMetadata = metadataEntries(selectedEntry?.raw?.metadata);
  $: selectedInputs = metadataEntries(selectedEntry?.raw?.inputs);
  $: selectedVariables = metadataEntries(selectedEntry?.raw?.variables);
  $: selectedRedactions = chipValues(selectedEntry?.raw?.contextRedactions);

  onMount(async () => {
    await loadKinds();
    await loadLogs();
  });
</script>

<div class="activity-log-viewer">
  <div class="page-header">
    <button type="button" on:click={() => dispatch('backToTerminals')} class="back-button">{$t('activityLog.back')}</button>
    <div class="page-actions">
      <button type="button" class="secondary-button" on:click={loadLogs} disabled={loading}>
        {loading ? $t('activityLog.refreshing') : $t('activityLog.refresh')}
      </button>
    </div>
  </div>

  <h2>{$t('activityLog.title')}</h2>
  <p class="intro-text">{$t('activityLog.intro')}</p>

  {#if errorMessage}
    <div class="error-message">{errorMessage}</div>
  {/if}

  <div class="log-toolbar">
    <label class="toolbar-field">
      <span>{$t('activityLog.kind')}</span>
      <select bind:value={selectedKind} on:change={loadLogs}>
        <option value="all">{$t('activityLog.all')}</option>
        {#each kinds as kind (kind)}
          <option value={kind}>{kindLabel(kind)}</option>
        {/each}
      </select>
    </label>
    <label class="toolbar-field">
      <span>{$t('activityLog.limit')}</span>
      <select bind:value={limit} on:change={loadLogs}>
        <option value={100}>100</option>
        <option value={200}>200</option>
        <option value={500}>500</option>
      </select>
    </label>
    <label class="toolbar-field toolbar-search">
      <span>{$t('activityLog.search')}</span>
      <input type="text" bind:value={searchQuery} placeholder={$t('activityLog.searchPlaceholder')} />
    </label>
  </div>

  <div class="log-layout">
    <div class="log-list">
      <div class="log-list-meta">{$t('activityLog.entries', { n: visibleLogs.length })}</div>
      {#if visibleLogs.length === 0}
        <div class="empty-state-card">{$t('activityLog.noEntries')}</div>
      {:else}
        {#each visibleLogs as entry (`${entry.kind}-${entry.timestamp}-${entry.title}`)}
          <button
            type="button"
            class="log-card"
            class:active-log={selectedEntry && selectedEntry.kind === entry.kind && selectedEntry.timestamp === entry.timestamp && selectedEntry.title === entry.title}
            on:click={() => selectEntry(entry)}
          >
            <div class="log-card-top">
              <span class="log-kind-pill">{kindLabel(entry.kind)}</span>
              <span class="log-time">{formatTimestamp(entry.timestamp)}</span>
            </div>
            <strong>{entry.title}</strong>
            <small>{entry.summary || $t('activityLog.noSummary')}</small>
          </button>
        {/each}
      {/if}
    </div>

    <div class="log-detail">
      {#if selectedEntry}
        <div class="detail-head">
          <div>
            <h3>{selectedEntry.title}</h3>
            <span>{kindLabel(selectedEntry.kind)} · {formatTimestamp(selectedEntry.timestamp)}</span>
          </div>
          <button type="button" class="secondary-button" on:click={copySelectedJSON}>{$t('activityLog.copyJSON')}</button>
        </div>
        <p class="detail-summary">{selectedEntry.summary || $t('activityLog.noSummary')}</p>

        {#if copyMessage}
          <div class="inline-note">{copyMessage}</div>
        {/if}

        <div class="detail-grid">
          {#each detailPairs(selectedEntry) as [label, value] (`${selectedEntry.kind}-${label}`)}
            <div class="detail-card">
              <span>{label}</span>
              <strong>{value}</strong>
            </div>
          {/each}
        </div>

        {#if selectedMetadata.length > 0}
          <section class="detail-section">
            <h4>{$t('activityLog.metadata')}</h4>
            <div class="chip-list">
              {#each selectedMetadata as [key, value] (`meta-${key}`)}
                <span>{key}: {value}</span>
              {/each}
            </div>
          </section>
        {/if}

        {#if selectedInputs.length > 0}
          <section class="detail-section">
            <h4>{$t('activityLog.inputs')}</h4>
            <div class="chip-list">
              {#each selectedInputs as [key, value] (`input-${key}`)}
                <span>{key}: {value}</span>
              {/each}
            </div>
          </section>
        {/if}

        {#if selectedVariables.length > 0}
          <section class="detail-section">
            <h4>{$t('activityLog.variables')}</h4>
            <div class="chip-list">
              {#each selectedVariables as [key, value] (`var-${key}`)}
                <span>{key}: {value}</span>
              {/each}
            </div>
          </section>
        {/if}

        {#if selectedRedactions.length > 0}
          <section class="detail-section">
            <h4>{$t('activityLog.redactions')}</h4>
            <div class="chip-list">
              {#each selectedRedactions as value (`redaction-${value}`)}
                <span>{value}</span>
              {/each}
            </div>
          </section>
        {/if}

        {#if selectedEntry.raw?.response}
          <section class="detail-section">
            <h4>{$t('activityLog.response')}</h4>
            <pre>{selectedEntry.raw.response}</pre>
          </section>
        {/if}

        {#if selectedEntry.raw?.prompt}
          <section class="detail-section">
            <h4>{$t('activityLog.promptRequest')}</h4>
            <pre>{selectedEntry.raw.prompt}</pre>
          </section>
        {/if}

        {#if selectedEntry.raw?.terminalOutput}
          <section class="detail-section">
            <h4>{$t('activityLog.sanitizedContext')}</h4>
            <pre>{selectedEntry.raw.terminalOutput}</pre>
          </section>
        {/if}

        <section class="detail-section">
          <h4>{$t('activityLog.rawPayload')}</h4>
          <pre>{JSON.stringify(selectedEntry.raw, null, 2)}</pre>
        </section>
      {:else}
        <div class="empty-state-card">{$t('activityLog.selectEntry')}</div>
      {/if}
    </div>
  </div>
</div>

<style>
  .activity-log-viewer {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    padding: 1.4rem;
    height: 100%;
    overflow: hidden;
    background: linear-gradient(180deg, rgba(17, 20, 32, 0.96), rgba(12, 14, 20, 0.98));
  }

  .page-header,
  .page-actions,
  .log-toolbar,
  .log-card-top,
  .detail-head {
    display: flex;
    align-items: center;
  }

  .page-header,
  .log-card-top,
  .detail-head {
    justify-content: space-between;
  }

  .page-actions,
  .log-toolbar {
    gap: 0.8rem;
  }

  .intro-text {
    margin: 0;
    color: var(--text-secondary);
  }

  .back-button,
  .secondary-button,
  .log-card,
  .toolbar-field input,
  .toolbar-field select {
    font: inherit;
  }

  .back-button,
  .secondary-button {
    border: 1px solid var(--border-dim);
    background: var(--bg-surface);
    color: var(--text-primary);
    border-radius: var(--radius-md);
    padding: 0.65rem 0.9rem;
    cursor: pointer;
    transition: border-color var(--transition), background var(--transition), transform var(--transition);
  }

  .back-button:hover,
  .secondary-button:hover,
  .log-card:hover {
    border-color: var(--border-accent);
    background: var(--bg-raised);
  }

  .log-toolbar {
    flex-wrap: wrap;
    padding: 0.9rem;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: rgba(22, 25, 38, 0.75);
  }

  .toolbar-field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    min-width: 140px;
  }

  .toolbar-field span {
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-secondary);
  }

  .toolbar-field input,
  .toolbar-field select {
    border: 1px solid var(--border-dim);
    background: var(--bg-deep);
    color: var(--text-primary);
    border-radius: var(--radius-md);
    padding: 0.65rem 0.75rem;
  }

  .toolbar-search {
    flex: 1 1 320px;
  }

  .log-layout {
    display: grid;
    grid-template-columns: minmax(320px, 0.95fr) minmax(0, 1.35fr);
    gap: 1rem;
    min-height: 0;
    flex: 1;
  }

  .log-list,
  .log-detail {
    min-height: 0;
    overflow: auto;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: rgba(17, 20, 32, 0.8);
    padding: 0.9rem;
  }

  .log-list {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
  }

  .log-list-meta {
    font-size: 0.82rem;
    color: var(--text-secondary);
  }

  .log-card {
    width: 100%;
    text-align: left;
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: rgba(22, 25, 38, 0.92);
    color: var(--text-primary);
    padding: 0.95rem;
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    cursor: pointer;
    transition: border-color var(--transition), background var(--transition), transform var(--transition);
  }

  .log-card strong {
    font-size: 0.96rem;
  }

  .log-card small,
  .detail-head span,
  .detail-summary {
    color: var(--text-secondary);
  }

  .inline-note {
    margin: 0 0 0.8rem;
    color: var(--accent-hover);
    font-size: 0.78rem;
  }

  .active-log {
    border-color: var(--accent);
    box-shadow: 0 0 0 1px rgba(99, 179, 237, 0.22);
    background: rgba(28, 32, 51, 0.98);
  }

  .log-kind-pill {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 0.2rem 0.55rem;
    background: rgba(99, 179, 237, 0.12);
    color: var(--accent-hover);
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  .log-time {
    font-size: 0.78rem;
    color: var(--text-muted);
  }

  .detail-head h3 {
    margin: 0 0 0.3rem;
  }

  .detail-summary {
    margin: 0.9rem 0 1rem;
  }

  .detail-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 0.75rem;
    margin-bottom: 1rem;
  }

  .detail-card,
  .detail-section {
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-lg);
    background: rgba(22, 25, 38, 0.72);
  }

  .detail-card {
    padding: 0.85rem;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .detail-card span,
  .detail-section h4 {
    color: var(--text-secondary);
    font-size: 0.74rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }

  .detail-card strong {
    font-size: 0.93rem;
    word-break: break-word;
  }

  .detail-section {
    padding: 0.9rem;
    margin-bottom: 1rem;
  }

  .detail-section h4 {
    margin: 0 0 0.75rem;
  }

  .chip-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.45rem;
  }

  .chip-list span {
    display: inline-flex;
    align-items: center;
    border-radius: 999px;
    padding: 0.2rem 0.55rem;
    background: rgba(99, 179, 237, 0.12);
    color: var(--accent-hover);
    font-size: 0.72rem;
  }

  .log-detail pre {
    margin: 0;
    padding: 1rem;
    border-radius: var(--radius-lg);
    background: var(--bg-void);
    border: 1px solid var(--border-subtle);
    color: var(--text-primary);
    font-family: var(--font-mono);
    font-size: 0.83rem;
    overflow: auto;
  }

  .empty-state-card,
  .error-message {
    border-radius: var(--radius-lg);
    padding: 1rem;
  }

  .empty-state-card {
    border: 1px dashed var(--border-dim);
    color: var(--text-secondary);
    background: rgba(22, 25, 38, 0.55);
  }

  .error-message {
    border: 1px solid rgba(244, 112, 103, 0.2);
    background: rgba(244, 112, 103, 0.1);
    color: #ffb4ac;
  }

  @media (max-width: 980px) {
    .log-layout {
      grid-template-columns: 1fr;
    }
  }
</style>
