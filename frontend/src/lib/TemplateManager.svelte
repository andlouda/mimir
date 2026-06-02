<script>
  import { createEventDispatcher } from 'svelte';
  import { t } from './i18n.js';

  export let SaveTemplate;
  export let UpdateTemplate;
  export let DeleteTemplate;
  export let ToggleFavorite;
  export let templates = [];
  export let templateToEdit = null;

  const dispatch = createEventDispatcher();

  const terminalTypeLabels = {
    bash: 'Bash',
    zsh: 'Zsh',
    wsl: 'WSL',
    cmd: 'CMD',
    powershell: 'PowerShell',
  };

  const categoryOrder = ['Navigation', 'Network', 'AI', 'Kubernetes', 'Containers', 'Windows', 'Linux', 'Storage', 'System', 'Diagnostics', 'General'];

  let reactivityCounter = 0;
  let templateName = '';
  let templateDescription = '';
  let templateCategory = 'General';
  let bashCommand = '';
  let zshCommand = '';
  let wslCommand = '';
  let cmdCommand = '';
  let powershellCommand = '';
  let templateParameters = [];
  let templateToolEnabled = true;
  let templateDangerLevel = 'low';
  let isFavorite = false;
  let errorMessage = '';
  let successMessage = '';
  let showForm = false;
  let collapsedCategories = {};
  let categoryViews = [];
  let visibleTemplateCount = 0;
  let templateSearchQuery = '';

  function normalizeTemplate(template) {
    return {
      name: template?.name || 'Unnamed Template',
      description: template?.description || '',
      category: template?.category || 'General',
      commands: template?.commands || {},
      parameters: Array.isArray(template?.parameters) ? template.parameters : [],
      toolEnabled: template?.toolEnabled ?? true,
      dangerLevel: template?.dangerLevel || 'low',
      favorite: Boolean(template?.favorite),
    };
  }

  function getCommandEntries(template) {
    return Object.entries(template.commands || {}).filter(([, command]) => command && command.trim() !== '');
  }

  function escapeHtml(value) {
    return String(value)
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;')
      .replaceAll('"', '&quot;')
      .replaceAll("'", '&#39;');
  }

  function fuzzyScore(value, query) {
    const source = (value || '').toLowerCase();
    if (!source || !query) {
      return 0;
    }

    if (source.includes(query)) {
      return 1000 - source.indexOf(query);
    }

    let score = 0;
    let queryIndex = 0;
    let streak = 0;

    for (let i = 0; i < source.length && queryIndex < query.length; i += 1) {
      if (source[i] === query[queryIndex]) {
        queryIndex += 1;
        streak += 1;
        score += 5 + streak;
      } else {
        streak = 0;
      }
    }

    if (queryIndex !== query.length) {
      return 0;
    }

    return score;
  }

  function getTemplateSearchScore(template, query) {
    if (!query) {
      return 1;
    }

    const values = [
      template.name,
      template.description,
      template.category,
      ...Object.values(template.commands || {}),
    ];

    let best = 0;
    for (const value of values) {
      const score = fuzzyScore(value, query);
      if (score > best) {
        best = score;
      }
    }
    return best;
  }

  function buildHighlightRanges(value, query) {
    if (!value || !query) {
      return [];
    }

    const source = value.toLowerCase();
    const directIndex = source.indexOf(query);
    if (directIndex !== -1) {
      return [{ start: directIndex, end: directIndex + query.length }];
    }

    const ranges = [];
    let queryIndex = 0;
    for (let i = 0; i < source.length && queryIndex < query.length; i += 1) {
      if (source[i] === query[queryIndex]) {
        ranges.push({ start: i, end: i + 1 });
        queryIndex += 1;
      }
    }

    if (queryIndex !== query.length) {
      return [];
    }

    return ranges;
  }

  function highlightText(value, query) {
    const safeValue = value || '';
    const ranges = buildHighlightRanges(safeValue, query);
    if (ranges.length === 0) {
      return escapeHtml(safeValue);
    }

    let result = '';
    let cursor = 0;
    for (const range of ranges) {
      if (range.start > cursor) {
        result += escapeHtml(safeValue.slice(cursor, range.start));
      }
      result += `<mark>${escapeHtml(safeValue.slice(range.start, range.end))}</mark>`;
      cursor = range.end;
    }
    if (cursor < safeValue.length) {
      result += escapeHtml(safeValue.slice(cursor));
    }
    return result;
  }

  function sortCategories(categories) {
    return [...categories].sort((a, b) => {
      const aIndex = categoryOrder.indexOf(a);
      const bIndex = categoryOrder.indexOf(b);
      const normalizedA = aIndex === -1 ? Number.MAX_SAFE_INTEGER : aIndex;
      const normalizedB = bIndex === -1 ? Number.MAX_SAFE_INTEGER : bIndex;
      if (normalizedA !== normalizedB) {
        return normalizedA - normalizedB;
      }
      return a.localeCompare(b);
    });
  }

  function getCategoryId(category) {
    return `category-${category.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`;
  }

  function isCategoryCollapsed(category) {
    return collapsedCategories[category] === true;
  }

  function scrollToCategory(category) {
    const element = document.getElementById(getCategoryId(category));
    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  }

  function setAllCategoriesCollapsed(collapsed) {
    const nextState = {};
    for (const view of categoryViews) {
      nextState[view.category] = collapsed;
    }
    collapsedCategories = nextState;
  }

  function toggleCategory(category) {
    collapsedCategories = {
      ...collapsedCategories,
      [category]: !(collapsedCategories[category] === true),
    };
  }

  $: {
    const query = templateSearchQuery.trim().toLowerCase();
    const nextCategoryMap = {};
    let nextVisibleCount = 0;

    for (const rawTemplate of templates || []) {
      const template = normalizeTemplate(rawTemplate);
      const searchScore = getTemplateSearchScore(template, query);
      if (searchScore === 0) {
        continue;
      }
      nextVisibleCount += 1;
      if (!nextCategoryMap[template.category]) {
        nextCategoryMap[template.category] = [];
      }
      nextCategoryMap[template.category].push({
        ...template,
        searchScore,
      });
    }
    for (const category of Object.keys(nextCategoryMap)) {
      nextCategoryMap[category].sort((a, b) => {
        if (b.searchScore !== a.searchScore) {
          return b.searchScore - a.searchScore;
        }
        return a.name.localeCompare(b.name);
      });
    }
    categoryViews = sortCategories(Object.keys(nextCategoryMap)).map((category) => ({
      category,
      templates: nextCategoryMap[category],
    }));
    visibleTemplateCount = nextVisibleCount;

    if (query === '') {
      const nextState = { ...collapsedCategories };
      let changed = false;
      for (const view of categoryViews) {
        if (!(view.category in nextState)) {
          nextState[view.category] = false;
          changed = true;
        }
      }
      if (changed) {
        collapsedCategories = nextState;
      }
    }
  }

  function syncBashCommands(event) {
    const value = event.target.value;
    bashCommand = value;
    zshCommand = value;
    wslCommand = value;
  }

  function clearFormFields() {
    templateName = '';
    templateDescription = '';
    templateCategory = 'General';
    bashCommand = '';
    zshCommand = '';
    wslCommand = '';
    cmdCommand = '';
    powershellCommand = '';
    templateParameters = [];
    templateToolEnabled = true;
    templateDangerLevel = 'low';
    isFavorite = false;
  }

  function resetForm() {
    clearFormFields();
    templateToEdit = null;
    showForm = false;
  }

  $: if (templateToEdit) {
    const normalizedTemplate = normalizeTemplate(templateToEdit);
    templateName = normalizedTemplate.name;
    templateDescription = normalizedTemplate.description;
    templateCategory = normalizedTemplate.category;
    bashCommand = normalizedTemplate.commands.bash || '';
    zshCommand = normalizedTemplate.commands.zsh || '';
    wslCommand = normalizedTemplate.commands.wsl || '';
    cmdCommand = normalizedTemplate.commands.cmd || '';
    powershellCommand = normalizedTemplate.commands.powershell || '';
    templateParameters = normalizedTemplate.parameters;
    templateToolEnabled = normalizedTemplate.toolEnabled;
    templateDangerLevel = normalizedTemplate.dangerLevel;
    isFavorite = normalizedTemplate.favorite;
    showForm = true;
  }

  function startCreateTemplate() {
    templateToEdit = null;
    clearFormFields();
    errorMessage = '';
    successMessage = '';
    showForm = true;
  }

  async function handleSubmit() {
    errorMessage = '';
    successMessage = '';

    if (!templateName) {
      errorMessage = 'Template Name is required.';
      return;
    }

    if (!templateToEdit && templates.some((template) => template.name === templateName)) {
      errorMessage = `A template with the name "${templateName}" already exists.`;
      return;
    }

    const newTemplate = {
      name: templateName,
      description: templateDescription,
      commands: {
        bash: bashCommand,
        zsh: zshCommand,
        wsl: wslCommand,
        cmd: cmdCommand,
        powershell: powershellCommand,
      },
      parameters: templateParameters,
      toolEnabled: templateToolEnabled,
      dangerLevel: templateDangerLevel,
      category: templateCategory,
      favorite: isFavorite,
    };

    try {
      let updatedTemplates;
      if (templateToEdit) {
        updatedTemplates = await (UpdateTemplate || window['go']['main']['App']['UpdateTemplate'])(JSON.stringify(newTemplate));
        successMessage = 'Template updated successfully!';
      } else {
        updatedTemplates = await (SaveTemplate || window['go']['main']['App']['SaveTemplate'])(JSON.stringify(newTemplate));
        successMessage = 'Template saved successfully!';
      }
      resetForm();
      dispatch('templateUpdated', updatedTemplates);
    } catch (error) {
      errorMessage = `Failed to save template: ${error.message || error}`;
      console.error('Save/Update template error:', error);
    }
  }

  async function handleDelete(name) {
    if (confirm(`Are you sure you want to delete template "${name}"?`)) {
      try {
        const updatedTemplates = await (DeleteTemplate || window['go']['main']['App']['DeleteTemplate'])(name);
        successMessage = `Template "${name}" deleted successfully!`;
        dispatch('templateUpdated', updatedTemplates);
      } catch (error) {
        errorMessage = `Failed to delete template: ${error.message || error}`;
        console.error('Delete template error:', error);
      }
    }
  }

  async function toggleFavorite(template) {
    try {
      let toggleFunc;
      if (typeof ToggleFavorite === 'function') {
        toggleFunc = ToggleFavorite;
      } else if (window['go'] && window['go']['main'] && window['go']['main']['App'] && window['go']['main']['App']['ToggleFavorite']) {
        toggleFunc = window['go']['main']['App']['ToggleFavorite'];
      } else {
        throw new Error('ToggleFavorite API not available');
      }

      const updatedTemplates = await toggleFunc(template.name);
      reactivityCounter++;
      successMessage = `Template "${template.name}" favorite status updated!`;
      dispatch('templateUpdated', updatedTemplates);
    } catch (error) {
      errorMessage = `Failed to update favorite status: ${error.message || error}`;
      console.error('ToggleFavorite error:', error);
    }
  }

  function handleEdit(template) {
    dispatch('editTemplate', template);
  }

  function goBackToTerminals() {
    dispatch('backToTerminals');
  }
</script>

<div class="template-manager">
  <div class="page-header">
    <button type="button" on:click={goBackToTerminals} class="back-button">{$t('templateManager.backToTerminals')}</button>
    <button type="button" on:click={startCreateTemplate} class="create-button">{$t('templateManager.newTemplate')}</button>
  </div>

  <h2>{$t('templateManager.title')}</h2>
  <p class="intro-text">{$t('templateManager.intro')}</p>

  <div class="template-search-row">
    <input
      type="text"
      bind:value={templateSearchQuery}
      class="template-search-input"
      placeholder={$t('templateManager.search')}
      aria-label={$t('templateManager.searchAria')}
    />
  </div>

  {#if errorMessage}
    <div class="error-message">{errorMessage}</div>
  {/if}
  {#if successMessage}
    <div class="success-message">{successMessage}</div>
  {/if}

  {#if templates.length === 0}
    <p>{$t('templateManager.emptyNone')}</p>
  {:else if categoryViews.length === 0}
    <p>{$t('templateManager.emptyFiltered')}</p>
  {:else}
    <nav class="category-toc" aria-label={$t('templateManager.tocAria')}>
      <div class="category-toc-header">
        <span class="category-toc-title">{$t('templateManager.toc')}</span>
        <div class="category-toc-actions">
          <button type="button" class="toc-action-button" on:click={() => setAllCategoriesCollapsed(true)}>{$t('templateManager.collapseAll')}</button>
          <button type="button" class="toc-action-button" on:click={() => setAllCategoriesCollapsed(false)}>{$t('templateManager.expandAll')}</button>
        </div>
      </div>
      <div class="category-search-status">
        {$t('templateManager.matches', { n: visibleTemplateCount })}
      </div>
      <div class="category-toc-list">
        {#each categoryViews as view (view.category)}
          <button type="button" class="category-chip" on:click={() => scrollToCategory(view.category)}>
            <span>{view.category}</span>
            <span class="category-chip-count">{view.templates.length}</span>
          </button>
        {/each}
      </div>
    </nav>

    {#each categoryViews as view (view.category)}
      <section
        class="category-section category-details"
        id={getCategoryId(view.category)}
      >
        <button
          type="button"
          class="category-header category-toggle"
          aria-expanded={!isCategoryCollapsed(view.category)}
          on:click={() => toggleCategory(view.category)}
        >
          <span class="category-header-main">
            <span class="category-arrow">{isCategoryCollapsed(view.category) ? '▸' : '▾'}</span>
            <span class="category-title">{view.category}</span>
          </span>
          <span class="category-count">{$t('templateManager.templatesCount', { n: view.templates.length })}</span>
        </button>

        {#if templateSearchQuery.trim() !== '' || !isCategoryCollapsed(view.category)}
          <div class="template-grid">
            {#each view.templates as template, index}
              <article class="template-card">
                <div class="template-card-header">
                  <div class="template-card-title-wrap">
                    <strong>{@html highlightText(template.name, templateSearchQuery.trim().toLowerCase())}</strong>
                    {#if template.favorite}
                      <span class="favorite-pill">{$t('templateManager.favorite')}</span>
                    {/if}
                  </div>
                  <div class="template-actions">
                    <button
                      type="button"
                      on:click|stopPropagation={() => toggleFavorite(template)}
                      class="favorite-button {template.favorite ? 'favorite-active' : 'favorite-inactive'}"
                      title={template.favorite ? $t('templateManager.removeFavorite') : $t('templateManager.addFavorite')}
                    >
                      {#key `${template.name}-${template.favorite}-${reactivityCounter}`}
                        {template.favorite ? '★' : '☆'}
                      {/key}
                    </button>
                    <button type="button" on:click|stopPropagation={() => handleEdit(template)} class="edit-button">{$t('templateManager.edit')}</button>
                    <button type="button" on:click|stopPropagation={() => handleDelete(template.name)} class="delete-button">{$t('templateManager.delete')}</button>
                  </div>
                </div>

                <p class="template-description">
                  {@html highlightText(template.description || $t('templateManager.noDescription'), templateSearchQuery.trim().toLowerCase())}
                </p>

                <div class="command-list">
                  {#each getCommandEntries(template) as [terminalType, command] (`${template.name}-${terminalType}-${index}`)}
                    <div class="command-tile">
                      <span class="command-shell">{terminalTypeLabels[terminalType] || terminalType}</span>
                      <code>{@html highlightText(command, templateSearchQuery.trim().toLowerCase())}</code>
                    </div>
                  {/each}
                </div>
              </article>
            {/each}
          </div>
        {/if}
      </section>
    {/each}
  {/if}

  {#if showForm}
    <section class="editor-panel">
      <h2>{templateToEdit ? $t('templateManager.editTitle') : $t('templateManager.createTitle')}</h2>

      <form on:submit|preventDefault={handleSubmit}>
        <div class="form-group">
          <label for="templateName">{$t('templateManager.name')}</label>
          <input type="text" id="templateName" bind:value={templateName} required />
        </div>

        <div class="form-group">
          <label for="templateDescription">{$t('templateManager.description')}</label>
          <textarea id="templateDescription" bind:value={templateDescription}></textarea>
        </div>

        <div class="form-group">
          <label for="templateCategory">{$t('templateManager.category')}</label>
          <select id="templateCategory" bind:value={templateCategory}>
            {#each categoryOrder as category (category)}
              <option value={category}>{category}</option>
            {/each}
          </select>
        </div>

        <h3>{$t('templateManager.commandsHeader')}</h3>
        <div class="form-group">
          <label for="bashCommand">{$t('templateManager.bashLabel')}</label>
          <input type="text" id="bashCommand" bind:value={bashCommand} on:input={syncBashCommands} placeholder={$t('templateManager.bashPlaceholder')} />
        </div>
        <div class="form-group">
          <label for="cmdCommand">{$t('templateManager.cmdLabel')}</label>
          <input type="text" id="cmdCommand" bind:value={cmdCommand} placeholder={$t('templateManager.cmdPlaceholder')} />
        </div>
        <div class="form-group">
          <label for="powershellCommand">{$t('templateManager.powershellLabel')}</label>
          <input type="text" id="powershellCommand" bind:value={powershellCommand} placeholder={$t('templateManager.powershellPlaceholder')} />
        </div>

        <button type="submit">{templateToEdit ? $t('templateManager.update') : $t('templateManager.save')}</button>
        <button type="button" on:click={resetForm} class="cancel-button">{templateToEdit ? $t('templateManager.cancelEdit') : $t('templateManager.close')}</button>
      </form>
    </section>
  {/if}
</div>
