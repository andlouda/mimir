<script>
  import { t } from '../i18n.js';

  export let lastRunState = null;
  export let approvalStep = null;
  export let approvalLoading = false;
  export let approvalDecisionMessage = '';
  export let close = () => {};
  export let approvePendingStep = () => {};
  export let denyPendingStep = () => {};
</script>

{#if lastRunState?.pendingApproval}
  <div
    class="approval-overlay"
    role="button"
    tabindex="0"
    on:click={() => !approvalLoading && close()}
    on:keydown={(e) => { if (e.key === 'Escape' && !approvalLoading) close(); }}
  >
    <div class="approval-modal" role="dialog" aria-modal="true" tabindex="-1" on:click|stopPropagation on:keydown|stopPropagation>
      <div class="pane-header">
        <div>
          <h3>{$t('workflowBuilder.approveStep')}</h3>
          <span>{lastRunState.pendingApproval.stepId}</span>
        </div>
        <button type="button" class="secondary-button" on:click={close} disabled={approvalLoading}>{$t('workflowBuilder.close')}</button>
      </div>

      <div class="approval-summary-grid">
        <div class="detail-card">
          <span>{$t('workflowBuilder.tool')}</span>
          <strong>{lastRunState.pendingApproval.toolName}</strong>
        </div>
        <div class="detail-card">
          <span>{$t('workflowBuilder.risk')}</span>
          <strong>{lastRunState.pendingApproval.risk}</strong>
        </div>
        <div class="detail-card">
          <span>{$t('workflowBuilder.reason')}</span>
          <strong>{lastRunState.pendingApproval.reason || $t('workflowBuilder.approvalRequired')}</strong>
        </div>
      </div>

      {#if approvalStep}
        <section class="detail-section">
          <h4>{$t('workflowBuilder.plannedStep')}</h4>
          <div class="chip-list">
            <span>{approvalStep.type}</span>
            <span>{approvalStep.functionName}</span>
            {#if approvalStep.tool}
              <span>{approvalStep.tool}</span>
            {/if}
            {#if approvalStep.discoveryTool}
              <span>{approvalStep.discoveryTool}</span>
            {/if}
          </div>
        </section>

        {#if Object.keys(approvalStep.inputs || {}).length > 0}
          <section class="detail-section">
            <h4>{$t('workflowBuilder.resolvedInputs')}</h4>
            <div class="chip-list">
              {#each Object.entries(approvalStep.inputs) as [key, value] (`approval-input-${key}`)}
                <span>{key}: {value || $t('workflowBuilder.emptyValue')}</span>
              {/each}
            </div>
          </section>
        {/if}

        {#if approvalStep.prompt}
          <section class="detail-section">
            <h4>{$t('workflowBuilder.prompt')}</h4>
            <pre>{approvalStep.prompt}</pre>
          </section>
        {/if}
      {/if}

      {#if approvalDecisionMessage}
        <div class="inline-note">{approvalDecisionMessage}</div>
      {/if}

      <div class="approval-actions">
        <button type="button" class="secondary-button danger-button" on:click={denyPendingStep} disabled={approvalLoading}>
          {approvalLoading ? $t('workflowBuilder.working') : $t('workflowBuilder.deny')}
        </button>
        <button type="button" class="secondary-button approve-button" on:click={approvePendingStep} disabled={approvalLoading}>
          {approvalLoading ? $t('workflowBuilder.working') : $t('workflowBuilder.approveContinue')}
        </button>
      </div>
    </div>
  </div>
{/if}
