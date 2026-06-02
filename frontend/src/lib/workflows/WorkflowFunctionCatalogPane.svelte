<script>
  import { t } from '../i18n.js';

  export let filteredFunctions = [];
  export let functionSearchQuery = '';
  export let addFunctionStep = () => {};
</script>

<section class="catalog-pane">
  <div class="pane-header">
    <h3>{$t('workflowBuilder.functionCatalog')}</h3>
    <span>{$t('workflowBuilder.functionsCount', { n: filteredFunctions.length })}</span>
  </div>
  <input
    type="text"
    bind:value={functionSearchQuery}
    class="search-input"
    placeholder={$t('workflowBuilder.searchFunctions')}
  />
  <div class="catalog-list">
    {#each filteredFunctions as entry (entry.id)}
      <article class="catalog-item">
        <div class="catalog-item-head">
          <strong>{entry.name}</strong>
          <span>{entry.category}</span>
        </div>
        <p>{entry.description}</p>
        {#if entry.parameters.length > 0}
          <div class="parameter-tags">
            {#each entry.parameters as parameter (`${entry.id}-${parameter.name}`)}
              <span>{parameter.name}{parameter.required ? '*' : ''}{parameter.source === 'discovery_only' ? ' · discovery' : ''}</span>
            {/each}
          </div>
        {/if}
        <div class="parameter-tags">
          <span>{entry.kind.replaceAll('_', ' ')}</span>
          {#if entry.discoveryTool}
            <span>{entry.discoveryTool}</span>
          {/if}
        </div>
        <button type="button" class="add-button" on:click={() => addFunctionStep(entry)}>
          {$t('workflowBuilder.addStep')}
        </button>
      </article>
    {/each}
  </div>
</section>
