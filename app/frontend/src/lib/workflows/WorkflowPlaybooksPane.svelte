<script>
  import { t } from '../i18n.js';

  export let visiblePlaybooks = [];
  export let playbookSearchQuery = '';
  export let prettyMode = (mode) => mode;
  export let startCustomWorkflow = () => {};
  export let loadPlaybook = () => {};
  export let runPlaybook = () => {};
</script>

<section class="playbook-hero">
  <div>
    <h3>{$t('workflowBuilder.heroTitle')}</h3>
    <p>{$t('workflowBuilder.heroDesc')}</p>
  </div>
  <button type="button" class="secondary-button" on:click={startCustomWorkflow}>{$t('workflowBuilder.startEmpty')}</button>
</section>

<section class="playbooks-pane">
  <div class="pane-header">
    <h3>{$t('workflowBuilder.troubleshootingPlaybooks')}</h3>
    <span>{$t('workflowBuilder.visible', { n: visiblePlaybooks.length })}</span>
  </div>
  <input
    type="text"
    bind:value={playbookSearchQuery}
    class="search-input"
    placeholder={$t('workflowBuilder.searchPlaybooks')}
  />
  <div class="playbook-list">
    {#each visiblePlaybooks as playbook (playbook.id)}
      <article class="playbook-card">
        <div class="catalog-item-head">
          <strong>{playbook.name}</strong>
          <span>{prettyMode(playbook.mode)}</span>
        </div>
        <p>{playbook.description}</p>
        <div class="parameter-tags">
          <span>{$t('workflowBuilder.stepsCount', { n: playbook.steps.length })}</span>
          <span>{$t('workflowBuilder.discoveryCount', { n: (playbook.steps || []).filter((step) => step.type === 'run_discovery').length })}</span>
          <span>{$t('workflowBuilder.aiCount', { n: (playbook.steps || []).filter((step) => step.type === 'ask_ai').length })}</span>
          {#if playbook.protected}
            <span>{$t('workflowBuilder.protectedTag')}</span>
          {/if}
        </div>
        <div class="playbook-step-outline">
          {#each playbook.steps.slice(0, 4) as step (`${playbook.id}-${step.id}`)}
            <div class="outline-row">
              <span class="outline-kind">{step.type.replaceAll('_', ' ')}</span>
              <span class="outline-name">{step.tool || step.discoveryTool || step.prompt || step.id}</span>
            </div>
          {/each}
        </div>
        <div class="playbook-actions">
          <button type="button" class="add-button" on:click={() => runPlaybook(playbook)}>{$t('workflowBuilder.runPlaybook')}</button>
          <button type="button" class="secondary-button" on:click={() => loadPlaybook(playbook)}>{$t('workflowBuilder.openInBuilder')}</button>
        </div>
      </article>
    {/each}
  </div>
</section>
