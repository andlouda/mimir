<script>
  import { t } from '../i18n.js';

  export let lastRunState = null;
  export let showJSONPreview = false;
  export let showMermaidPreview = false;
  export let exportWorkflowJSON = () => '';
  export let exportWorkflowMermaid = () => '';
</script>

<section class="run-summary-grid">
  <article class="summary-card">
    <span class="status-label">{$t('workflowBuilder.runStatus')}</span>
    <strong>{lastRunState ? (lastRunState.pendingApproval ? $t('workflowBuilder.waitingApproval') : $t('workflowBuilder.latestRunAvailable')) : $t('workflowBuilder.noRunYet')}</strong>
    <small>{lastRunState?.workflowId || $t('workflowBuilder.runDraftHint')}</small>
  </article>
  <article class="summary-card">
    <span class="status-label">{$t('workflowBuilder.discoveryValuesLabel')}</span>
    <strong>{lastRunState?.discovery ? Object.values(lastRunState.discovery).reduce((sum, values) => sum + values.length, 0) : 0}</strong>
    <small>{$t('workflowBuilder.collectedAcross')}</small>
  </article>
  <article class="summary-card">
    <span class="status-label">{$t('workflowBuilder.events')}</span>
    <strong>{lastRunState?.events?.length || 0}</strong>
    <small>{$t('workflowBuilder.timelineEntries')}</small>
  </article>
</section>

{#if lastRunState?.pendingApproval}
  <section class="approval-card">
    <div>
      <h3>{$t('workflowBuilder.approvalNeeded')}</h3>
      <p>{lastRunState.pendingApproval.reason || $t('workflowBuilder.approvalNeededDesc')}</p>
    </div>
    <div class="parameter-tags">
      <span>{lastRunState.pendingApproval.toolName}</span>
      <span>{lastRunState.pendingApproval.risk}</span>
      <span>{lastRunState.pendingApproval.stepId}</span>
    </div>
  </section>
{/if}

<section class="preview-pane">
  <div class="pane-header">
    <h3>{$t('workflowBuilder.replayTimeline')}</h3>
    <span>{$t('workflowBuilder.eventsCount', { n: lastRunState?.events?.length || 0 })}</span>
  </div>
  {#if lastRunState?.events?.length}
    <div class="event-timeline">
      {#each lastRunState.events as event, index (`${event.stepId || 'workflow'}-${event.type}-${index}`)}
        <article class="timeline-card">
          <div class="catalog-item-head">
            <strong>{event.type.replaceAll('_', ' ')}</strong>
            <span>{event.stepId || 'workflow'}</span>
          </div>
          <p>{event.message || $t('workflowBuilder.noMessage')}</p>
          {#if event.metadata && Object.keys(event.metadata).length > 0}
            <div class="result-chip-list">
              {#each Object.entries(event.metadata) as [key, value] (`${event.type}-${key}`)}
                <span>{key}: {value}</span>
              {/each}
            </div>
          {/if}
        </article>
      {/each}
    </div>
  {:else}
    <div class="empty-state-card">{$t('workflowBuilder.replayHint')}</div>
  {/if}
</section>

<section class="preview-pane">
  <div class="pane-header">
    <h3>{$t('workflowBuilder.jsonPreview')}</h3>
    <button type="button" class="secondary-button" on:click={() => showJSONPreview = !showJSONPreview}>
      {showJSONPreview ? $t('workflowBuilder.hideJSON') : $t('workflowBuilder.showJSON')}
    </button>
  </div>
  {#if showJSONPreview}
    <pre>{exportWorkflowJSON()}</pre>
  {/if}
</section>

<section class="preview-pane">
  <div class="pane-header">
    <h3>{$t('workflowBuilder.mermaid')}</h3>
    <button type="button" class="secondary-button" on:click={() => showMermaidPreview = !showMermaidPreview}>
      {showMermaidPreview ? $t('workflowBuilder.hideMermaid') : $t('workflowBuilder.showMermaid')}
    </button>
  </div>
  {#if showMermaidPreview}
    <pre>{exportWorkflowMermaid()}</pre>
  {/if}
</section>
