<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { t } from '../i18n.js';
  import WorkflowApprovalDialog from './WorkflowApprovalDialog.svelte';
  import WorkflowFunctionCatalogPane from './WorkflowFunctionCatalogPane.svelte';
  import WorkflowPlaybooksPane from './WorkflowPlaybooksPane.svelte';
  import WorkflowRunSummary from './WorkflowRunSummary.svelte';

  export let queuedCatalogEntry = null;
  export let queuedCatalogEntries = [];
  export let activeTerminalId = null;
  export let activeTerminalType = '';
  export let activeTerminalName = '';
  export let activeTerminalOutput = '';

  const dispatch = createEventDispatcher();
  const storageKey = 'mimir-workflow-draft-v1';

  let functionCatalog = [];
  let playbooks = [];
  let workflowView = 'playbooks';
  let playbookSearchQuery = '';
  let functionSearchQuery = '';
  let errorMessage = '';
  let successMessage = '';
  let workflowName = '';
  let workflowDescription = '';
  let workflowMode = 'assist';
  let workflowSteps = [];
  let loadedPlaybookId = '';
  let runLoading = false;
  let lastRunState = null;
  let lastQueuedSeedId = '';
  let lastQueuedBatchSignature = '';
  let showJSONPreview = false;
  let showMermaidPreview = false;
  let showApprovalDialog = false;
  let approvalLoading = false;
  let approvalDecisionMessage = '';

  function normalizeCatalogEntry(entry) {
    return {
      id: entry?.id || '',
      name: entry?.name || 'Unnamed Function',
      kind: entry?.kind || 'unknown',
      category: entry?.category || 'General',
      description: entry?.description || '',
      parameters: Array.isArray(entry?.parameters) ? entry.parameters : [],
      commands: entry?.commands || {},
      discoveryTool: entry?.discoveryTool || '',
    };
  }

  function normalizePlaybook(playbook) {
    return {
      id: playbook?.id || '',
      name: playbook?.name || 'Unnamed Playbook',
      description: playbook?.description || '',
      mode: playbook?.mode || 'assist',
      steps: Array.isArray(playbook?.steps) ? playbook.steps : [],
      protected: Boolean(playbook?.protected),
    };
  }

  function prettyMode(mode) {
    if (!mode) return 'assist';
    return mode.charAt(0).toUpperCase() + mode.slice(1);
  }

  function createWorkflowStep(entry) {
    const stepId = `step-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    const base = {
      id: stepId,
      functionId: entry.id,
      functionName: entry.name,
      category: entry.category,
      kind: entry.kind,
      description: entry.description,
      type: entry.kind === 'template_tool' ? 'run_tool' : entry.kind === 'discovery_tool' ? 'run_discovery' : 'ask_ai',
      tool: entry.kind === 'template_tool' ? entry.id : '',
      discoveryTool: entry.kind === 'discovery_tool' ? entry.discoveryTool || entry.id : '',
      aiMode: entry.kind === 'ai_action' ? entry.id.replace(/^ai:/, '') : '',
      prompt: '',
      requiresApproval: false,
      inputs: {},
    };

    for (const parameter of entry.parameters) {
      base.inputs[parameter.name] = '';
    }

    return base;
  }

  function saveDraft() {
    if (typeof localStorage === 'undefined') {
      return;
    }

    const payload = {
      workflowName,
      workflowDescription,
      workflowMode,
      workflowSteps,
      loadedPlaybookId,
    };
    localStorage.setItem(storageKey, JSON.stringify(payload));
  }

  function loadDraft() {
    if (typeof localStorage === 'undefined') {
      return;
    }

    const raw = localStorage.getItem(storageKey);
    if (!raw) {
      return;
    }

    try {
      const parsed = JSON.parse(raw);
      workflowName = parsed.workflowName || '';
      workflowDescription = parsed.workflowDescription || '';
      workflowMode = parsed.workflowMode || 'assist';
      workflowSteps = Array.isArray(parsed.workflowSteps) ? parsed.workflowSteps : [];
      loadedPlaybookId = parsed.loadedPlaybookId || '';
    } catch (error) {
      console.error('Failed to restore workflow draft', error);
    }
  }

  async function loadFunctionCatalog() {
    try {
      const payload = await window['go']['main']['App']['GetFunctionCatalogJSON']();
      functionCatalog = JSON.parse(payload).map(normalizeCatalogEntry);
      errorMessage = '';
    } catch (error) {
      errorMessage = `Failed to load function catalog: ${error.message || error}`;
    }
  }

  async function loadPlaybooks() {
    try {
      const payload = await window['go']['main']['App']['GetPlaybooksJSON']();
      playbooks = JSON.parse(payload).map(normalizePlaybook);
      errorMessage = '';
    } catch (error) {
      errorMessage = `Failed to load playbooks: ${error.message || error}`;
    }
  }

  function addFunctionStep(entry) {
    workflowSteps = [...workflowSteps, createWorkflowStep(entry)];
    workflowView = 'builder';
    successMessage = `${entry.name} added to workflow.`;
    errorMessage = '';
  }

  function moveStep(index, direction) {
    const targetIndex = index + direction;
    if (targetIndex < 0 || targetIndex >= workflowSteps.length) {
      return;
    }

    const next = [...workflowSteps];
    const current = next[index];
    next[index] = next[targetIndex];
    next[targetIndex] = current;
    workflowSteps = next;
  }

  function removeStep(index) {
    workflowSteps = workflowSteps.filter((_, stepIndex) => stepIndex !== index);
  }

  function resetWorkflow() {
    workflowName = '';
    workflowDescription = '';
    workflowMode = 'assist';
    workflowSteps = [];
    loadedPlaybookId = '';
    lastRunState = null;
    workflowView = 'builder';
    successMessage = 'Workflow draft reset.';
  }

  function loadPlaybook(playbook) {
    workflowView = 'builder';
    loadedPlaybookId = playbook.id || '';
    workflowName = playbook.name;
    workflowDescription = playbook.description;
    workflowMode = playbook.mode || 'assist';
    workflowSteps = (playbook.steps || []).map((step, index) => ({
      id: step.id || `step-${Date.now()}-${index}`,
      functionId: step.tool || step.discoveryTool || step.type || '',
      functionName: step.tool
        ? step.tool.replace(/^template:/, '')
        : step.discoveryTool
          ? step.discoveryTool.replace(/^discovery:/, 'Discovery: ')
          : (step.type === 'ask_ai' ? 'AI Step' : 'Workflow Step'),
      category: step.tool ? 'Playbook Tool' : step.discoveryTool ? 'Discovery' : 'AI',
      kind: step.tool ? 'template_tool' : step.discoveryTool ? 'discovery_tool' : 'ai_action',
      description: '',
      type: step.type || 'run_tool',
      tool: step.tool || '',
      aiMode: '',
      prompt: step.prompt || '',
      requiresApproval: Boolean(step.requiresApproval),
      inputs: step.inputs ? { ...step.inputs } : {},
      discoveryTool: step.discoveryTool || '',
    }));
    successMessage = `${playbook.name} loaded into the workflow draft.`;
    errorMessage = '';
  }

  async function savePlaybook() {
    if (!workflowName.trim()) {
      errorMessage = 'A playbook name is required before saving.';
      successMessage = '';
      return;
    }
    if (workflowSteps.length === 0) {
      errorMessage = 'Add at least one step before saving a playbook.';
      successMessage = '';
      return;
    }

    try {
      const definition = buildWorkflowDefinitionObject();
      const loadedPlaybook = playbooks.find((playbook) => playbook.id === loadedPlaybookId);
      const previousID = loadedPlaybook?.protected ? '' : (loadedPlaybookId || '');
      if (loadedPlaybook?.protected && definition.id === loadedPlaybookId) {
        definition.id = `${definition.id}-copy`;
      }

      const payload = await window['go']['main']['App']['SavePlaybookJSON'](
        JSON.stringify(definition, null, 2),
        previousID
      );
      const saved = normalizePlaybook(JSON.parse(payload));
      loadedPlaybookId = saved.id;
      await loadPlaybooks();
      successMessage = `${saved.name} saved as a playbook.`;
      errorMessage = '';
    } catch (error) {
      errorMessage = `Failed to save playbook: ${error.message || error}`;
      successMessage = '';
    }
  }

  async function deletePlaybook() {
    if (!loadedPlaybookId) {
      errorMessage = 'Load or save a playbook first before deleting it.';
      successMessage = '';
      return;
    }
    if (typeof window !== 'undefined' && typeof window.confirm === 'function' && !window.confirm(`Delete playbook "${workflowName || loadedPlaybookId}"?`)) {
      return;
    }

    try {
      await window['go']['main']['App']['DeletePlaybook'](loadedPlaybookId);
      const deletedName = workflowName || loadedPlaybookId;
      resetWorkflow();
      await loadPlaybooks();
      successMessage = `${deletedName} deleted.`;
      errorMessage = '';
    } catch (error) {
      errorMessage = `Failed to delete playbook: ${error.message || error}`;
      successMessage = '';
    }
  }

  function exportWorkflowJSON() {
    return JSON.stringify(buildWorkflowDefinitionObject(), null, 2);
  }

  function buildWorkflowDefinitionObject() {
    const workflowId = workflowName
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '') || 'workflow-draft';

    return {
      id: workflowId,
      name: workflowName || 'Untitled Workflow',
      description: workflowDescription,
      mode: workflowMode,
      steps: workflowSteps.map((step) => ({
        id: step.id,
        type: step.type,
        tool: step.tool,
        discoveryTool: step.discoveryTool || '',
        prompt: step.prompt || '',
        inputs: step.inputs || {},
        requiresApproval: Boolean(step.requiresApproval),
        aiMode: step.aiMode || '',
        functionId: step.functionId,
        functionName: step.functionName,
      })),
    };
  }

  function toMermaidLabel(step, index) {
    return `${index + 1}. ${step.functionName}`.replaceAll('"', '\'');
  }

  function exportWorkflowMermaid() {
    if (workflowSteps.length === 0) {
      return 'flowchart LR\n  Start([Start]) --> End([End])';
    }

    const lines = ['flowchart LR'];
    lines.push('  Start([Start])');
    workflowSteps.forEach((step, index) => {
      const nodeId = `S${index + 1}`;
      const shapeStart = step.kind === 'ai_action' ? '{{' : '[';
      const shapeEnd = step.kind === 'ai_action' ? '}}' : ']';
      lines.push(`  ${nodeId}${shapeStart}"${toMermaidLabel(step, index)}"${shapeEnd}`);
      if (step.requiresApproval) {
        lines.push(`  A${index + 1}{"Approval"}`);
      }
    });

    lines.push('  End([End])');
    workflowSteps.forEach((step, index) => {
      const currentNode = `S${index + 1}`;
      const previousNode = index === 0 ? 'Start' : (workflowSteps[index - 1].requiresApproval ? `A${index}` : `S${index}`);
      lines.push(`  ${previousNode} --> ${currentNode}`);
      if (step.requiresApproval) {
        lines.push(`  ${currentNode} --> A${index + 1}`);
      }
    });
    const lastNode = workflowSteps[workflowSteps.length - 1].requiresApproval ? `A${workflowSteps.length}` : `S${workflowSteps.length}`;
    lines.push(`  ${lastNode} --> End`);

    return lines.join('\n');
  }

  async function runPlaybook(playbook) {
    runLoading = true;
    try {
      const definition = JSON.stringify({
        id: playbook.id,
        name: playbook.name,
        description: playbook.description || '',
        mode: playbook.mode || 'assist',
        steps: playbook.steps || [],
      });
      const payload = await window['go']['main']['App']['RunWorkflowDraftJSON'](
        definition,
        Number(activeTerminalId || 0),
        activeTerminalType || '',
        activeTerminalName || '',
        activeTerminalOutput || ''
      );
      lastRunState = JSON.parse(payload);
      showApprovalDialog = Boolean(lastRunState?.pendingApproval);
      approvalDecisionMessage = '';
      successMessage = `Playbook "${playbook.name}" run completed.`;
      errorMessage = '';
      workflowView = 'builder';
      loadPlaybook(playbook);
    } catch (error) {
      lastRunState = null;
      errorMessage = `Failed to run playbook: ${error.message || error}`;
      successMessage = '';
    } finally {
      runLoading = false;
    }
  }

  async function runWorkflowDraft() {
    runLoading = true;
    try {
      const payload = await window['go']['main']['App']['RunWorkflowDraftJSON'](
        exportWorkflowJSON(),
        Number(activeTerminalId || 0),
        activeTerminalType || '',
        activeTerminalName || '',
        activeTerminalOutput || ''
      );
      lastRunState = JSON.parse(payload);
      showApprovalDialog = Boolean(lastRunState?.pendingApproval);
      approvalDecisionMessage = '';
      successMessage = 'Workflow run completed.';
      errorMessage = '';
    } catch (error) {
      lastRunState = null;
      errorMessage = `Failed to run workflow draft: ${error.message || error}`;
      successMessage = '';
    } finally {
      runLoading = false;
    }
  }

  async function approvePendingStep() {
    if (!lastRunState?.pendingApproval) {
      return;
    }

    approvalLoading = true;
    try {
      const payload = await window['go']['main']['App']['ResumeWorkflowDraftJSON'](
        exportWorkflowJSON(),
        JSON.stringify(lastRunState),
        Number(activeTerminalId || 0),
        activeTerminalType || '',
        activeTerminalName || '',
        activeTerminalOutput || ''
      );
      lastRunState = JSON.parse(payload);
      showApprovalDialog = Boolean(lastRunState?.pendingApproval);
      approvalDecisionMessage = lastRunState?.pendingApproval
        ? 'Step approved. The workflow paused again for the next approval.'
        : 'Step approved and workflow continued.';
      successMessage = approvalDecisionMessage;
      errorMessage = '';
    } catch (error) {
      errorMessage = `Failed to approve workflow step: ${error.message || error}`;
      successMessage = '';
    } finally {
      approvalLoading = false;
    }
  }

  async function denyPendingStep() {
    if (!lastRunState?.pendingApproval) {
      return;
    }

    approvalLoading = true;
    try {
      const payload = await window['go']['main']['App']['RejectWorkflowDraftJSON'](
        JSON.stringify(lastRunState),
        'approval denied by user'
      );
      lastRunState = JSON.parse(payload);
      showApprovalDialog = false;
      approvalDecisionMessage = 'Step denied. The workflow remained stopped.';
      successMessage = approvalDecisionMessage;
      errorMessage = '';
    } catch (error) {
      errorMessage = `Failed to deny workflow step: ${error.message || error}`;
      successMessage = '';
    } finally {
      approvalLoading = false;
    }
  }

  function startCustomWorkflow() {
    resetWorkflow();
    successMessage = '';
    errorMessage = '';
    workflowView = 'builder';
  }

  function openPlaybooksView() {
    workflowView = 'playbooks';
  }

  function openBuilderView() {
    workflowView = 'builder';
  }

  function summarizeStep(step) {
    if (step.kind === 'discovery_tool') {
      return step.discoveryTool || step.functionName;
    }
    if (step.kind === 'ai_action') {
      return step.prompt || step.functionName;
    }
    return step.tool || step.functionName;
  }

  function stepEventFor(stepId, type) {
    return lastRunState?.events?.find((event) => event.stepId === stepId && event.type === type) || null;
  }

  function discoveryValues(stepId) {
    return Array.isArray(lastRunState?.discovery?.[stepId]) ? lastRunState.discovery[stepId] : [];
  }

  function outputFor(stepId) {
    return lastRunState?.outputs?.[stepId] || '';
  }

  function pendingApprovalStep() {
    if (!lastRunState?.pendingApproval?.stepId) {
      return null;
    }
    return workflowSteps.find((step) => step.id === lastRunState.pendingApproval.stepId) || null;
  }

  function isLoadedPlaybookProtected() {
    return Boolean(playbooks.find((playbook) => playbook.id === loadedPlaybookId)?.protected);
  }

  function runStatusFor(step) {
    if (!lastRunState) {
      return 'idle';
    }
    if (lastRunState?.pendingApproval?.stepId === step.id) {
      return 'approval';
    }
    if (stepEventFor(step.id, 'step_completed')) {
      return 'completed';
    }
    if (stepEventFor(step.id, 'step_failed')) {
      return 'failed';
    }
    if (stepEventFor(step.id, 'step_started')) {
      return 'running';
    }
    return 'idle';
  }

  function runStatusLabel(step) {
    const status = runStatusFor(step);
    switch (status) {
      case 'completed': return $t('workflowBuilder.statusCompleted');
      case 'running': return $t('workflowBuilder.statusRunning');
      case 'approval': return $t('workflowBuilder.statusNeedsApproval');
      case 'failed': return $t('workflowBuilder.statusFailed');
      default: return $t('workflowBuilder.statusPending');
    }
  }

  $: filteredFunctions = functionCatalog.filter((entry) => {
    const query = functionSearchQuery.trim().toLowerCase();
    if (!query) {
      return true;
    }

    const searchable = [
      entry.name,
      entry.kind,
      entry.category,
      entry.description,
      ...(entry.parameters || []).map((parameter) => parameter.name),
    ].join(' ').toLowerCase();

    return searchable.includes(query);
  });

  $: visiblePlaybooks = playbooks.filter((playbook) => {
    const query = playbookSearchQuery.trim().toLowerCase();
    if (!query) {
      return true;
    }
    return [
      playbook.name,
      playbook.description,
      playbook.mode,
      ...(playbook.steps || []).map((step) => `${step.type} ${step.tool || ''} ${step.discoveryTool || ''} ${step.prompt || ''}`),
    ].join(' ').toLowerCase().includes(query);
  });

  $: if (!lastRunState?.pendingApproval) {
    showApprovalDialog = false;
  }

  $: if (queuedCatalogEntry && queuedCatalogEntry._seedId && queuedCatalogEntry._seedId !== lastQueuedSeedId) {
    addFunctionStep(normalizeCatalogEntry(queuedCatalogEntry));
    lastQueuedSeedId = queuedCatalogEntry._seedId;
  }

  $: {
    const batchSignature = Array.isArray(queuedCatalogEntries)
      ? queuedCatalogEntries.map((entry) => entry?._seedId || '').join('|')
      : '';

    if (batchSignature && batchSignature !== lastQueuedBatchSignature) {
      for (const entry of queuedCatalogEntries) {
        addFunctionStep(normalizeCatalogEntry(entry));
      }
      lastQueuedBatchSignature = batchSignature;
    }
  }

  $: saveDraft();

  onMount(async () => {
    loadDraft();
    await loadFunctionCatalog();
    await loadPlaybooks();
  });
</script>

<div class="workflow-builder">
  <div class="page-header">
    <button type="button" on:click={() => dispatch('backToTerminals')} class="back-button">{$t('workflowBuilder.back')}</button>
    <div class="page-actions">
      <button type="button" class="secondary-button" on:click={openPlaybooksView}>
        {$t('workflowBuilder.playbooks')}
      </button>
      <button type="button" class="secondary-button" on:click={openBuilderView}>
        {$t('workflowBuilder.builder')}
      </button>
      <button type="button" class="secondary-button" on:click={runWorkflowDraft} disabled={runLoading || workflowSteps.length === 0}>
        {runLoading ? $t('workflowBuilder.running') : $t('workflowBuilder.runDraft')}
      </button>
      <button type="button" class="secondary-button" on:click={savePlaybook}>
        {isLoadedPlaybookProtected() ? $t('workflowBuilder.saveAsCopy') : (loadedPlaybookId ? $t('workflowBuilder.updatePlaybook') : $t('workflowBuilder.saveAsPlaybook'))}
      </button>
      <button type="button" class="secondary-button danger-button" on:click={deletePlaybook} disabled={!loadedPlaybookId || isLoadedPlaybookProtected()}>
        {$t('workflowBuilder.deletePlaybook')}
      </button>
      <button type="button" class="secondary-button" on:click={resetWorkflow}>{$t('workflowBuilder.resetDraft')}</button>
    </div>
  </div>

  <h2>{$t('workflowBuilder.title')}</h2>
  <p class="intro-text">{$t('workflowBuilder.intro')}</p>

  {#if errorMessage}
    <div class="error-message">{errorMessage}</div>
  {/if}
  {#if successMessage}
    <div class="success-message">{successMessage}</div>
  {/if}

  <section class="mode-switch-card">
    <button type="button" class:active-switch={workflowView === 'playbooks'} on:click={openPlaybooksView}>{$t('workflowBuilder.playbooksFirst')}</button>
    <button type="button" class:active-switch={workflowView === 'builder'} on:click={openBuilderView}>{$t('workflowBuilder.customBuilder')}</button>
    <button type="button" class="secondary-button ghost-button" on:click={startCustomWorkflow}>{$t('workflowBuilder.newCustom')}</button>
  </section>

  <div class="workflow-meta-card">
    <label class="form-field">
      <span>{$t('workflowBuilder.name')}</span>
      <input type="text" bind:value={workflowName} placeholder={$t('workflowBuilder.namePlaceholder')} />
    </label>
    <label class="form-field">
      <span>{$t('workflowBuilder.description')}</span>
      <textarea bind:value={workflowDescription} placeholder={$t('workflowBuilder.descriptionPlaceholder')}></textarea>
    </label>
    <label class="form-field narrow">
      <span>{$t('workflowBuilder.mode')}</span>
      <select bind:value={workflowMode}>
        <option value="manual">{$t('workflowBuilder.modeManual')}</option>
        <option value="assist">{$t('workflowBuilder.modeAssist')}</option>
        <option value="approve">{$t('workflowBuilder.modeApprove')}</option>
        <option value="auto">{$t('workflowBuilder.modeAuto')}</option>
      </select>
    </label>
    <div class="playbook-status">
      <span class="status-label">{$t('workflowBuilder.playbookId')}</span>
      <strong>{loadedPlaybookId || $t('workflowBuilder.notSaved')}</strong>
      {#if isLoadedPlaybookProtected()}
        <small class="protected-note">{$t('workflowBuilder.protectedNote')}</small>
      {/if}
    </div>
  </div>

  {#if workflowView === 'playbooks'}
    <WorkflowPlaybooksPane
      {visiblePlaybooks}
      bind:playbookSearchQuery
      {prettyMode}
      {startCustomWorkflow}
      {loadPlaybook}
      {runPlaybook}
    />
  {/if}

  {#if workflowView === 'builder'}
  <div class="workflow-layout">
    <WorkflowFunctionCatalogPane
      {filteredFunctions}
      bind:functionSearchQuery
      {addFunctionStep}
    />

    <section class="steps-pane">
      <div class="pane-header">
        <h3>{$t('workflowBuilder.workflowSteps')}</h3>
        <span>{$t('workflowBuilder.stepsCount', { n: workflowSteps.length })}</span>
      </div>
      {#if workflowSteps.length === 0}
        <div class="empty-state">{$t('workflowBuilder.emptySteps')}</div>
      {:else}
        <div class="step-list">
          {#each workflowSteps as step, index (step.id)}
            <article class="step-card">
              <div class="step-card-header">
                <div>
                  <strong>{index + 1}. {step.functionName}</strong>
                  <div class="step-meta">{step.category} · {step.kind} · {step.type}</div>
                  <div class="step-status-row">
                    <span class:status-pill={true} class:status-completed={runStatusFor(step) === 'completed'} class:status-running={runStatusFor(step) === 'running'} class:status-approval={runStatusFor(step) === 'approval'}>{runStatusLabel(step)}</span>
                    {#if step.kind === 'discovery_tool'}
                      <span class="mini-note">{$t('workflowBuilder.valuesCount', { n: discoveryValues(step.id).length })}</span>
                    {:else if outputFor(step.id)}
                      <span class="mini-note">{outputFor(step.id).slice(0, 72)}{outputFor(step.id).length > 72 ? '…' : ''}</span>
                    {/if}
                  </div>
                </div>
                <div class="step-actions">
                  <button type="button" on:click={() => moveStep(index, -1)} disabled={index === 0}>↑</button>
                  <button type="button" on:click={() => moveStep(index, 1)} disabled={index === workflowSteps.length - 1}>↓</button>
                  <button type="button" on:click={() => removeStep(index)}>{$t('workflowBuilder.remove')}</button>
                </div>
              </div>

              {#if step.kind === 'ai_action'}
                <label class="form-field">
                  <span>{$t('workflowBuilder.promptGoal')}</span>
                  <textarea bind:value={step.prompt} placeholder={$t('workflowBuilder.promptPlaceholder')}></textarea>
                </label>
              {/if}

              {#if Object.keys(step.inputs || {}).length > 0}
                <div class="step-input-grid">
                  {#each Object.keys(step.inputs) as inputKey (`${step.id}-${inputKey}`)}
                    <label class="form-field">
                      <span>{inputKey}</span>
                      <input type="text" bind:value={workflowSteps[index].inputs[inputKey]} placeholder={inputKey} />
                    </label>
                  {/each}
                </div>
              {/if}

              <label class="approval-toggle">
                <input type="checkbox" bind:checked={workflowSteps[index].requiresApproval} />
                <span>{$t('workflowBuilder.requireApproval')}</span>
              </label>

              {#if discoveryValues(step.id).length > 0}
                <div class="result-card">
                  <strong>{$t('workflowBuilder.discoveryResults')}</strong>
                  <div class="result-chip-list">
                    {#each discoveryValues(step.id).slice(0, 16) as value (`${step.id}-${value}`)}
                      <span>{value}</span>
                    {/each}
                  </div>
                </div>
              {/if}

              {#if outputFor(step.id) && step.kind !== 'discovery_tool'}
                <div class="result-card">
                  <strong>{$t('workflowBuilder.latestOutput')}</strong>
                  <pre>{outputFor(step.id)}</pre>
                </div>
              {/if}
            </article>
          {/each}
        </div>
      {/if}
    </section>
  </div>
  {/if}

  {#if workflowView === 'builder'}
    <WorkflowRunSummary
      {lastRunState}
      bind:showJSONPreview
      bind:showMermaidPreview
      {exportWorkflowJSON}
      {exportWorkflowMermaid}
    />
  {/if}

  {#if showApprovalDialog && lastRunState?.pendingApproval}
    <WorkflowApprovalDialog
      {lastRunState}
      approvalStep={pendingApprovalStep()}
      {approvalLoading}
      {approvalDecisionMessage}
      close={() => { showApprovalDialog = false; }}
      {approvePendingStep}
      {denyPendingStep}
    />
  {/if}
</div>
