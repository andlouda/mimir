<script>
  import { onMount, onDestroy } from 'svelte';
  import { t } from '../i18n.js';
  import { agentAnnotations } from '../stores/agentStore.js';
  import { agentWorkspace, attentionItems, liveWorkspace, workspaceError, loadAgentWorkspace, saveWorkspaceProject, saveWorkspaceTask, linkWorkspaceSession, unlinkWorkspaceSession } from '../stores/agentWorkspaceStore.js';
  import { loadAgentSessions, openAgentPanel } from '../actions/agentActions.js';

  export let selectTerminal = () => {};
  let selectedProject = '';
  let tab = 'attention';
  let showArchived = false;
  let loading = true;
  let busy = false;
  let error = '';
  let editor = '';
  let draft = {};
  let linkProject = '';
  let linkTask = '';
  let linkTerminal = '';
  let linkFile = '';
  let candidates = [];
  let listing = false;
  let request = 0;
  const statuses = ['planned', 'in_progress', 'blocked', 'review', 'done'];

  $: projects = $agentWorkspace.projects.filter((p) => showArchived || !p.archived);
  $: project = $agentWorkspace.projects.find((p) => p.id === selectedProject);
  $: tasks = $agentWorkspace.tasks.filter((task) => (!selectedProject || task.projectId === selectedProject)
    && (showArchived || (!task.archived && !$agentWorkspace.projects.find((p) => p.id === task.projectId)?.archived)));
  $: sessions = $agentWorkspace.sessions.filter((ref) => (!selectedProject || ref.projectId === selectedProject)
    && (showArchived || (!$agentWorkspace.projects.find((p) => p.id === ref.projectId)?.archived
      && !$agentWorkspace.tasks.find((task) => task.id === ref.taskId)?.archived)));
  $: attention = $attentionItems.filter((item) => !selectedProject || item.projectId === selectedProject);
  $: linkable = $liveWorkspace.filter((row) => row.agent.transcripts);
  $: linkTasks = $agentWorkspace.tasks.filter((task) => task.projectId === linkProject && !task.archived);

  onMount(refresh);
  onDestroy(() => { request++; });

  async function refresh() {
    loading = true;
    try { await loadAgentWorkspace(); } catch { /* persisted error shown below */ }
    finally { loading = false; }
  }
  async function mutate(fn) {
    if (busy) return;
    busy = true;
    error = '';
    try { await fn(); }
    catch (e) { error = String(e?.message || e); }
    finally { busy = false; }
  }
  function closeEditor() {
    editor = '';
    request++;
    listing = false;
    error = '';
  }
  function editProject(value = null) {
    closeEditor();
    draft = value ? { ...value } : { name: '', description: '', host: 'local', root: '', archived: false };
    editor = 'project';
  }
  function editTask(value = null) {
    closeEditor();
    draft = value ? { ...value } : { projectId: project && !project.archived ? project.id : $agentWorkspace.projects.find((p) => !p.archived)?.id || '', title: '', description: '', status: 'planned', archived: false };
    editor = 'task';
  }
  async function saveDraft() {
    await mutate(async () => {
      if (editor === 'project') {
        await saveWorkspaceProject(draft);
        if (draft.archived && selectedProject === draft.id && !showArchived) selectedProject = '';
      }
      else await saveWorkspaceTask(draft);
      closeEditor();
    });
  }
  function startLink(task = null) {
    closeEditor();
    editor = 'link';
    linkProject = task?.projectId || (project && !project.archived ? project.id : $agentWorkspace.projects.find((p) => !p.archived)?.id || '');
    linkTask = task?.id || '';
    linkTerminal = '';
    linkFile = '';
    candidates = [];
  }
  async function listSessions() {
    const token = ++request;
    linkFile = '';
    candidates = [];
    error = '';
    if (!linkTerminal) { listing = false; return; }
    listing = true;
    try {
      const data = await loadAgentSessions(Number(linkTerminal));
      if (token !== request) return;
      candidates = data.sessions || [];
      // A current, discovered session is safe to preselect; never guess by mtime.
      const current = data.selected || linkable.find((row) => row.id === Number(linkTerminal))?.agent.sessionFile;
      if (candidates.some((s) => s.file === current)) linkFile = current;
    } catch (e) {
      if (token === request) error = String(e?.message || e);
    } finally { if (token === request) listing = false; }
  }
  async function assign() {
    await mutate(async () => {
      const row = linkable.find((r) => r.id === Number(linkTerminal));
      if (!row) throw new Error($t('agentWorkspace.noLiveSession'));
      await linkWorkspaceSession(linkProject, linkTask, row.terminal, linkFile);
      closeEditor();
      tab = 'sessions';
      selectedProject = linkProject;
    });
  }
  function projectName(id) { return $agentWorkspace.projects.find((p) => p.id === id)?.name || ''; }
  function taskName(id) { return $agentWorkspace.tasks.find((task) => task.id === id)?.title || ''; }
  function sessionName(ref) { return $agentAnnotations.sessions[`${ref.host}|${ref.file}`]?.name || ref.title || ref.file.split(/[\\/]/).pop(); }
  function liveFor(ref) { return $liveWorkspace.find((row) => row.session?.id === ref.id); }
  function openLive(row) {
    openAgentPanel(row.id);
    selectTerminal(row.terminal);
  }
</script>

<section class="workspace" aria-label={$t('agentWorkspace.title')}>
  <header class="workspace-header">
    <div class="workspace-title">
      <h1>{$t('agentWorkspace.title')}</h1>
      <p>{$t('agentWorkspace.subtitle')}</p>
    </div>
    <div class="workspace-header-actions">
      <label class="check"><input type="checkbox" bind:checked={showArchived} on:change={() => { if (!showArchived && project?.archived) selectedProject = ''; }} />{$t('agentWorkspace.showArchived')}</label>
      <button class="quiet" disabled={loading || busy} on:click={refresh}>{$t('agentWorkspace.refresh')}</button>
    </div>
  </header>
  {#if error || $workspaceError}<div class="error" role="alert">{error || $workspaceError}</div>{/if}
  {#if loading}<p class="status" role="status">{$t('agentWorkspace.loading')}</p>{/if}

  <nav class="project-row" aria-label={$t('agentWorkspace.projects')}>
    <button class="project-button" class:chosen={!selectedProject} on:click={() => { selectedProject = ''; }}>{$t('agentWorkspace.allProjects')}</button>
    {#each projects as item (item.id)}
      <button class="project-button" class:chosen={selectedProject === item.id} class:muted={item.archived} on:click={() => { selectedProject = item.id; }} title={item.description || item.root || ''}>
        {item.name}{#if item.host && item.host !== 'local'}<span class="chip-meta">{item.host}</span>{/if}{#if item.archived}<span class="chip-meta">{$t('agentWorkspace.archived')}</span>{/if}
      </button>
    {/each}
    <button class="project-button project-add" disabled={busy || loading} on:click={() => editProject()} aria-label={$t('agentWorkspace.newProject')} title={$t('agentWorkspace.newProject')}>+ {$t('agentWorkspace.newProject')}</button>
  </nav>
  {#if !projects.length && !loading}<p class="hint">{$t('agentWorkspace.noProjects')}</p>{/if}

  {#if project}
    <div class="project-summary">
      <div class="project-summary-text">
        <h2>{project.name}</h2>
        {#if project.description}<p>{project.description}</p>{/if}
        <span class="meta"><span>{project.host}</span>{#if project.root}<span class="mono">{project.root}</span>{/if}</span>
      </div>
      <button class="quiet" disabled={busy} on:click={() => editProject(project)}>{$t('agentWorkspace.editProject')}</button>
    </div>
  {/if}

  {#if editor}
    <form class="editor" on:submit|preventDefault={editor === 'link' ? assign : saveDraft}>
      <h2>{$t(`agentWorkspace.${editor === 'project' ? (draft.id ? 'editProject' : 'newProject') : editor === 'task' ? (draft.id ? 'editTask' : 'newTask') : 'linkSession'}`)}</h2>
      <fieldset disabled={busy}>
      {#if editor === 'project'}
        <label>{$t('agentWorkspace.name')}<input required maxlength="120" bind:value={draft.name} /></label>
        <label>{$t('agentWorkspace.description')}<textarea maxlength="8000" rows="3" bind:value={draft.description}></textarea></label>
        <div class="form-row"><label>{$t('agentWorkspace.host')}<input required maxlength="512" bind:value={draft.host} /></label><label>{$t('agentWorkspace.directory')}<input maxlength="4096" bind:value={draft.root} placeholder="/path/to/project" /></label></div>
      {:else if editor === 'task'}
        <label>{$t('agentWorkspace.project')}<select required disabled={!!draft.id} bind:value={draft.projectId}><option value="">{$t('agentWorkspace.chooseProject')}</option>{#each $agentWorkspace.projects.filter((p) => !p.archived) as p}<option value={p.id}>{p.name}</option>{/each}</select></label>
        <label>{$t('agentWorkspace.taskTitle')}<input required maxlength="160" bind:value={draft.title} /></label>
        <label>{$t('agentWorkspace.description')}<textarea maxlength="8000" rows="4" bind:value={draft.description}></textarea></label>
        <label>{$t('agentWorkspace.status')}<select bind:value={draft.status}>{#each statuses as status}<option value={status}>{$t(`agentWorkspace.statuses.${status}`)}</option>{/each}</select></label>
      {:else}
        <p class="form-hint">{$t('agentWorkspace.linkHint')}</p>
        <div class="form-row">
          <label>{$t('agentWorkspace.project')}<select required bind:value={linkProject} on:change={() => { linkTask = ''; }}><option value="">{$t('agentWorkspace.chooseProject')}</option>{#each $agentWorkspace.projects.filter((p) => !p.archived) as p}<option value={p.id}>{p.name}</option>{/each}</select></label>
          <label>{$t('agentWorkspace.task')}<select bind:value={linkTask}><option value="">{$t('agentWorkspace.projectOnly')}</option>{#each linkTasks as task}<option value={task.id}>{task.title}</option>{/each}</select></label>
        </div>
        <label>{$t('agentWorkspace.terminal')}<select required bind:value={linkTerminal} on:change={listSessions}><option value="">{$t('agentWorkspace.chooseTerminal')}</option>{#each linkable as row}<option value={String(row.id)}>{row.terminal.name} — {row.agent.label} ({row.agent.project?.host || row.agent.source})</option>{/each}</select></label>
        {#if !linkable.length}<p class="form-hint">{$t('agentWorkspace.noLiveSession')}</p>{/if}
        {#if listing}<p class="status" role="status">{$t('agentWorkspace.loading')}</p>{/if}
        <label>{$t('agentWorkspace.session')}<select required disabled={listing || !candidates.length} bind:value={linkFile}><option value="">{$t('agentWorkspace.chooseSession')}</option>{#each candidates as candidate}<option value={candidate.file}>{candidate.title || candidate.file}{candidate.modified ? ` (${candidate.modified})` : ''}</option>{/each}</select></label>
        {#if linkTerminal && !listing && !candidates.length}<p class="form-hint">{$t('agentWorkspace.noSessionsFound')}</p>{/if}
        {#if linkFile}<span class="mono path">{linkFile}</span>{/if}
      {/if}
      {#if editor !== 'link' && draft.id}<label class="check"><input type="checkbox" bind:checked={draft.archived} />{$t('agentWorkspace.archived')}</label>{/if}
      <div class="actions"><button type="submit" class="primary" disabled={editor === 'link' && (!linkFile || !linkProject || listing)}>{$t(editor === 'link' ? 'agentWorkspace.assign' : 'agentWorkspace.save')}</button><button type="button" class="quiet" on:click={closeEditor}>{$t('agentWorkspace.cancel')}</button></div>
      </fieldset>
    </form>
  {/if}

  <nav class="workspace-tabs" aria-label={$t('agentWorkspace.views')}>
    <div class="segmented">
      <button aria-pressed={tab === 'attention'} class:chosen={tab === 'attention'} on:click={() => { tab = 'attention'; }}>{$t('agentWorkspace.attention')} <span>{attention.length}</span></button>
      <button aria-pressed={tab === 'tasks'} class:chosen={tab === 'tasks'} on:click={() => { tab = 'tasks'; }}>{$t('agentWorkspace.tasks')} <span>{tasks.length}</span></button>
      <button aria-pressed={tab === 'sessions'} class:chosen={tab === 'sessions'} on:click={() => { tab = 'sessions'; }}>{$t('agentWorkspace.sessions')} <span>{sessions.length}</span></button>
    </div>
    {#if tab === 'tasks'}
      <button class="primary" disabled={busy || loading || !$agentWorkspace.projects.some((p) => !p.archived) || project?.archived} on:click={() => editTask()}>{$t('agentWorkspace.newTask')}</button>
    {:else if tab === 'sessions'}
      <button class="primary" disabled={busy || loading || !$agentWorkspace.projects.some((p) => !p.archived) || project?.archived} on:click={() => startLink()}>{$t('agentWorkspace.linkSession')}</button>
    {/if}
  </nav>

  <div class="workspace-body">
    {#if tab === 'attention'}
      <p class="hint">{$t('agentWorkspace.attentionHint')}</p>
      {#if attention.length}
        <div class="list">
          {#each attention as item (item.key)}
            <article class="row attention-card" data-reason={item.reason}>
              <span class="stripe stripe-{item.reason}" aria-hidden="true"></span>
              <div class="row-main">
                <div class="row-head">
                  <h3>{item.task?.title || taskName(item.taskId) || item.agent?.title || item.agent?.label}</h3>
                  <span class="tag tag-{item.reason}">{$t(`agentWorkspace.reasons.${item.reason}`)}</span>
                </div>
                {#if item.task?.description || item.agent?.lastText || item.agent?.subject}<p class="row-text">{item.task?.description || item.agent?.lastText || item.agent?.subject}</p>{/if}
                <span class="meta"><span>{projectName(item.projectId) || $t('agentWorkspace.unassigned')}</span>{#if item.terminal}<span>{item.terminal.name}</span><span>{item.agent.label}</span>{/if}</span>
              </div>
              <div class="row-actions">{#if item.terminal}<button class="quiet" on:click={() => openLive(item)}>{$t('agentWorkspace.openTerminal')}</button>{/if}{#if item.task}<button class="quiet" disabled={busy} on:click={() => editTask(item.task)}>{$t('agentWorkspace.editTask')}</button>{/if}</div>
            </article>
          {/each}
        </div>
      {:else}
        <div class="empty-state"><h2>{$t('agentWorkspace.noAttention')}</h2><p>{$t('agentWorkspace.noAttentionHint')}</p></div>
      {/if}
    {:else if tab === 'tasks'}
      <p class="hint">{$t('agentWorkspace.taskHint')}</p>
      {#if tasks.length}
        <div class="list">
          {#each tasks as task (task.id)}
            <article class="row" data-testid="workspace-task">
              <span class="stripe stripe-{task.status}" aria-hidden="true"></span>
              <div class="row-main">
                <div class="row-head">
                  <h3>{task.title}</h3>
                  <span class="tag tag-{task.status}">{$t(`agentWorkspace.statuses.${task.status}`)}</span>
                  {#if task.archived}<span class="tag">{$t('agentWorkspace.archived')}</span>{/if}
                </div>
                {#if task.description}<p class="row-text">{task.description}</p>{/if}
                <span class="meta">
                  <span>{projectName(task.projectId)}</span>
                  {#each $agentWorkspace.sessions.filter((ref) => ref.taskId === task.id) as ref}
                    <span class="session-ref" class:live={!!liveFor(ref)}><span class="dot" aria-hidden="true"></span>{sessionName(ref)}</span>
                  {/each}
                </span>
              </div>
              <div class="row-actions"><button class="quiet" disabled={busy || $agentWorkspace.projects.find((p) => p.id === task.projectId)?.archived} on:click={() => editTask(task)}>{$t('agentWorkspace.editTask')}</button><button class="quiet" disabled={busy || task.archived || $agentWorkspace.projects.find((p) => p.id === task.projectId)?.archived} on:click={() => startLink(task)}>{$t('agentWorkspace.linkSession')}</button></div>
            </article>
          {/each}
        </div>
      {:else}
        <div class="empty-state"><h2>{$t('agentWorkspace.noTasks')}</h2><p>{$t('agentWorkspace.noTasksHint')}</p></div>
      {/if}
    {:else}
      <p class="hint">{$t('agentWorkspace.sessionHint')}</p>
      {#if sessions.length}
        <div class="list">
          {#each sessions as ref (ref.id)}
            {@const live = liveFor(ref)}
            <article class="row" data-testid="workspace-session">
              <span class="stripe" class:stripe-live={!!live} aria-hidden="true"></span>
              <div class="row-main">
                <div class="row-head">
                  <h3>{sessionName(ref)}</h3>
                  <span class="tag" class:tag-live={!!live}>{$t(`agentWorkspace.${live ? 'connected' : 'offline'}`)}</span>
                </div>
                <span class="meta"><span>{projectName(ref.projectId)}</span><span>{taskName(ref.taskId) || $t('agentWorkspace.projectOnly')}</span><span>{ref.kind}</span></span>
                <span class="mono path">{ref.host}: {ref.cwd}<br />{ref.file}</span>
              </div>
              <div class="row-actions">{#if live}<button class="quiet" on:click={() => openLive(live)}>{$t('agentWorkspace.openTerminal')}</button>{/if}<button class="quiet" disabled={busy} on:click={() => mutate(() => unlinkWorkspaceSession(ref.id))}>{$t('agentWorkspace.unlink')}</button></div>
            </article>
          {/each}
        </div>
      {:else}
        <div class="empty-state"><h2>{$t('agentWorkspace.noSessions')}</h2><p>{$t('agentWorkspace.sessionHint')}</p></div>
      {/if}
    {/if}
  </div>
</section>

<style>
  .workspace { height: 100%; overflow: auto; padding: 26px 32px 48px; box-sizing: border-box; color: var(--text-primary); max-width: 960px; font-size: 14px; line-height: 1.5; }
  .workspace-header { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; margin-bottom: 18px; }
  .workspace-header-actions { display: flex; align-items: center; gap: 14px; flex-shrink: 0; }
  h1 { font-size: 22px; margin: 0 0 2px; font-weight: 700; letter-spacing: -0.01em; }
  h2 { font-size: 16px; margin: 0; font-weight: 600; }
  h3 { font-size: 15px; margin: 0; font-weight: 600; line-height: 1.35; min-width: 0; overflow-wrap: anywhere; }
  p { color: var(--text-secondary); margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
  .hint { font-size: 13px; margin-bottom: 12px; }
  .status { margin-bottom: 12px; }
  .meta { display: flex; flex-wrap: wrap; gap: 4px 14px; font-size: 12.5px; color: var(--text-secondary); }
  .mono { font-family: var(--font-mono, monospace); font-size: 11.5px; }
  .path { display: block; color: var(--text-secondary); line-height: 1.6; margin-top: 4px; overflow-wrap: anywhere; }

  button, input, select, textarea { font: inherit; color: var(--text-primary); background: var(--bg-raised); border: 1px solid var(--border-dim); border-radius: var(--radius-md); padding: 7px 12px; }
  button { cursor: pointer; line-height: 1.3; white-space: nowrap; }
  button:hover:not(:disabled) { border-color: var(--accent); }
  button:disabled { opacity: .45; cursor: default; }
  button:focus-visible, input:focus-visible, select:focus-visible, textarea:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .quiet { background: transparent; }
  .primary { color: var(--accent); background: var(--accent-glow); border-color: var(--accent); }

  /* Projects as a row of chips; the chosen one carries the accent. */
  .project-row { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin-bottom: 16px; }
  .project-button { display: inline-flex; align-items: center; gap: 8px; padding: 5px 12px; border-radius: 999px; font-size: 13px; max-width: 320px; overflow: hidden; text-overflow: ellipsis; }
  .project-button.chosen { color: var(--accent); border-color: var(--accent); background: var(--accent-glow); }
  .project-button.muted { opacity: .6; }
  .chip-meta { font-size: 11.5px; opacity: .75; }
  .project-add { border-style: dashed; color: var(--text-secondary); background: transparent; }
  .project-summary { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 14px 18px; margin-bottom: 18px; background: var(--bg-surface); border: 1px solid var(--border-dim); border-radius: var(--radius-lg); }
  .project-summary-text { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .project-summary p { max-height: 120px; overflow: auto; }

  .workspace-tabs { display: flex; gap: 12px; flex-wrap: wrap; align-items: center; justify-content: space-between; margin: 4px 0 14px; }
  .segmented { display: inline-flex; padding: 3px; gap: 2px; background: var(--bg-surface); border: 1px solid var(--border-dim); border-radius: var(--radius-md); }
  .segmented button { background: transparent; border-color: transparent; padding: 5px 12px; color: var(--text-secondary); }
  .segmented button.chosen { background: var(--bg-raised); color: var(--text-primary); }
  .segmented span { margin-left: 6px; opacity: .7; font-variant-numeric: tabular-nums; }

  /* Rows: a status stripe, the text, the actions. Lines, not boxes. */
  .list { background: var(--bg-surface); border: 1px solid var(--border-dim); border-radius: var(--radius-lg); overflow: hidden; }
  .row { display: grid; grid-template-columns: 4px minmax(0, 1fr) auto; gap: 0 16px; align-items: start; padding: 14px 16px 14px 0; border-top: 1px solid var(--border-dim); }
  .row:first-child { border-top: 0; }
  .row:hover { background: var(--bg-raised); }
  .stripe { align-self: stretch; background: var(--border-dim); border-radius: 0 2px 2px 0; }
  .stripe-permission, .stripe-blocked { background: #ff7b72; }
  .stripe-input, .stripe-in_progress { background: var(--warning); }
  .stripe-result, .stripe-done, .stripe-live { background: #7ee787; }
  .stripe-review { background: #d2a8ff; }
  .stripe-planned { background: var(--accent); opacity: .6; }
  .row-main { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  .row-head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .row-text { font-size: 13.5px; max-height: 6.2em; overflow: auto; }
  .row-actions { display: flex; gap: 6px; flex-wrap: wrap; justify-content: flex-end; }
  .tag { display: inline-block; font-size: 11.5px; font-weight: 600; padding: 1px 8px; border-radius: 999px; color: var(--text-secondary); background: var(--bg-raised); white-space: nowrap; }
  .tag-permission, .tag-blocked { color: #ff7b72; background: rgba(255, 123, 114, 0.14); }
  .tag-input, .tag-in_progress { color: var(--warning); background: rgba(227, 179, 65, 0.14); }
  .tag-result, .tag-done, .tag-live { color: #7ee787; background: rgba(126, 231, 135, 0.14); }
  .tag-review { color: #d2a8ff; background: rgba(210, 168, 255, 0.14); }
  .tag-planned { color: var(--accent); background: var(--accent-glow); }
  .session-ref { display: inline-flex; align-items: center; gap: 6px; }
  .session-ref .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--border-dim); }
  .session-ref.live .dot { background: #7ee787; }

  .editor { background: var(--bg-surface); border: 1px solid var(--border-dim); border-left: 3px solid var(--accent); border-radius: var(--radius-lg); padding: 16px 20px; margin-bottom: 18px; max-width: 720px; }
  fieldset { border: 0; margin: 12px 0 0; padding: 0; min-width: 0; }
  fieldset label { display: flex; flex-direction: column; gap: 6px; margin-bottom: 12px; font-size: 13px; color: var(--text-secondary); }
  fieldset label input, fieldset label select, fieldset label textarea { color: var(--text-primary); }
  .form-hint { font-size: 13px; margin-bottom: 10px; }
  .check { display: flex; flex-direction: row; align-items: center; gap: 8px; font-size: 12.5px; color: var(--text-secondary); }
  .check input { width: auto; margin: 0; }
  input, select, textarea { box-sizing: border-box; width: 100%; }
  textarea { resize: vertical; }
  .form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  .form-row label { min-width: 0; }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 4px; }
  .empty-state { padding: 28px 4px; text-align: left; }
  .empty-state h2 { margin-bottom: 4px; }
  .error { background: var(--error-bg); color: var(--error); padding: 12px 14px; border-radius: var(--radius-md); overflow-wrap: anywhere; margin-bottom: 14px; }
  @media (max-width: 720px) {
    .workspace { padding: 16px; }
    .workspace-header { flex-direction: column; align-items: flex-start; }
    .form-row { grid-template-columns: 1fr; }
    .row { grid-template-columns: 4px minmax(0, 1fr); }
    .row-actions { grid-column: 2; justify-content: flex-start; margin-top: 8px; }
  }
</style>
