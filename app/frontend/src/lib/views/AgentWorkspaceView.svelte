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
      <label class="archive-toggle"><input type="checkbox" bind:checked={showArchived} on:change={() => { if (!showArchived && project?.archived) selectedProject = ''; }} />{$t('agentWorkspace.showArchived')}</label>
      <button disabled={loading || busy} on:click={refresh}>{$t('agentWorkspace.refresh')}</button>
    </div>
  </header>
  {#if error || $workspaceError}<div class="error" role="alert">{error || $workspaceError}</div>{/if}
  {#if loading}<p class="status" role="status">{$t('agentWorkspace.loading')}</p>{/if}

  <nav class="project-row" aria-label={$t('agentWorkspace.projects')}>
    <button class="project-button" class:chosen={!selectedProject} on:click={() => { selectedProject = ''; }}>{$t('agentWorkspace.allProjects')}</button>
    {#each projects as item (item.id)}
      <button class="project-button" class:chosen={selectedProject === item.id} class:muted={item.archived} on:click={() => { selectedProject = item.id; }} title={item.description || item.root || ''}>
        {item.name}{#if item.host && item.host !== 'local'}<small> · {item.host}</small>{/if}{#if item.archived}<small> · {$t('agentWorkspace.archived')}</small>{/if}
      </button>
    {/each}
    <button class="project-button project-add" disabled={busy || loading} on:click={() => editProject()} aria-label={$t('agentWorkspace.newProject')} title={$t('agentWorkspace.newProject')}>+ {$t('agentWorkspace.newProject')}</button>
  </nav>
  {#if !projects.length && !loading}<p class="empty">{$t('agentWorkspace.noProjects')}</p>{/if}

  {#if project}
    <div class="project-summary">
      <div class="project-summary-text">
        <h2>{project.name}</h2>
        {#if project.description}<p>{project.description}</p>{/if}
        <small>{project.host}{project.root ? ` · ${project.root}` : ''}</small>
      </div>
      <button disabled={busy} on:click={() => editProject(project)}>{$t('agentWorkspace.editProject')}</button>
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
        <label>{$t('agentWorkspace.terminal')}<select required bind:value={linkTerminal} on:change={listSessions}><option value="">{$t('agentWorkspace.chooseTerminal')}</option>{#each linkable as row}<option value={String(row.id)}>{row.terminal.name} · {row.agent.label} · {row.agent.project?.host || row.agent.source}</option>{/each}</select></label>
        {#if !linkable.length}<p class="form-hint">{$t('agentWorkspace.noLiveSession')}</p>{/if}
        {#if listing}<p class="status" role="status">{$t('agentWorkspace.loading')}</p>{/if}
        <label>{$t('agentWorkspace.session')}<select required disabled={listing || !candidates.length} bind:value={linkFile}><option value="">{$t('agentWorkspace.chooseSession')}</option>{#each candidates as candidate}<option value={candidate.file}>{candidate.title || candidate.file} · {candidate.modified || ''}</option>{/each}</select></label>
        {#if linkTerminal && !listing && !candidates.length}<p class="form-hint">{$t('agentWorkspace.noSessionsFound')}</p>{/if}
        {#if linkFile}<small class="session-path">{linkFile}</small>{/if}
      {/if}
      {#if editor !== 'link' && draft.id}<label class="archive-toggle"><input type="checkbox" bind:checked={draft.archived} />{$t('agentWorkspace.archived')}</label>{/if}
      <div class="actions"><button type="submit" class="primary" disabled={editor === 'link' && (!linkFile || !linkProject || listing)}>{$t(editor === 'link' ? 'agentWorkspace.assign' : 'agentWorkspace.save')}</button><button type="button" on:click={closeEditor}>{$t('agentWorkspace.cancel')}</button></div>
      </fieldset>
    </form>
  {/if}

  <nav class="workspace-tabs" aria-label={$t('agentWorkspace.views')}>
    <button class:chosen={tab === 'attention'} on:click={() => { tab = 'attention'; }}>{$t('agentWorkspace.attention')} <span>{attention.length}</span></button>
    <button class:chosen={tab === 'tasks'} on:click={() => { tab = 'tasks'; }}>{$t('agentWorkspace.tasks')} <span>{tasks.length}</span></button>
    <button class:chosen={tab === 'sessions'} on:click={() => { tab = 'sessions'; }}>{$t('agentWorkspace.sessions')} <span>{sessions.length}</span></button>
    {#if tab === 'tasks'}
      <button class="primary tab-action" disabled={busy || loading || !$agentWorkspace.projects.some((p) => !p.archived) || project?.archived} on:click={() => editTask()}>{$t('agentWorkspace.newTask')}</button>
    {:else if tab === 'sessions'}
      <button class="primary tab-action" disabled={busy || loading || !$agentWorkspace.projects.some((p) => !p.archived) || project?.archived} on:click={() => startLink()}>{$t('agentWorkspace.linkSession')}</button>
    {/if}
  </nav>

  <div class="workspace-body">
    {#if tab === 'attention'}
      <p class="view-hint">{$t('agentWorkspace.attentionHint')}</p>
      {#each attention as item (item.key)}
        <article class="card attention-card" class:urgent={item.reason === 'permission' || item.reason === 'blocked'}>
          <div class="card-top"><span class="badge badge-{item.reason}">{$t(`agentWorkspace.reasons.${item.reason}`)}</span><small>{projectName(item.projectId) || $t('agentWorkspace.unassigned')}</small></div>
          <h3>{item.task?.title || taskName(item.taskId) || item.agent?.title || item.agent?.label}</h3>
          {#if item.task?.description || item.agent?.lastText || item.agent?.subject}<p>{item.task?.description || item.agent?.lastText || item.agent?.subject}</p>{/if}
          {#if item.terminal}<small>{item.terminal.name} · {item.agent.label}</small>{/if}
          <div class="actions">{#if item.terminal}<button on:click={() => openLive(item)}>{$t('agentWorkspace.openTerminal')}</button>{/if}{#if item.task}<button disabled={busy} on:click={() => editTask(item.task)}>{$t('agentWorkspace.editTask')}</button>{/if}</div>
        </article>
      {:else}<div class="empty-state"><h2>{$t('agentWorkspace.noAttention')}</h2><p>{$t('agentWorkspace.noAttentionHint')}</p></div>{/each}
    {:else if tab === 'tasks'}
      <p class="view-hint">{$t('agentWorkspace.taskHint')}</p>
      {#each tasks as task (task.id)}
        <article class="card" data-testid="workspace-task">
          <div class="card-top"><span class="badge badge-{task.status}">{$t(`agentWorkspace.statuses.${task.status}`)}{task.archived ? ` · ${$t('agentWorkspace.archived')}` : ''}</span><small>{projectName(task.projectId)}</small></div>
          <h3>{task.title}</h3>
          {#if task.description}<p>{task.description}</p>{/if}
          {#if $agentWorkspace.sessions.some((ref) => ref.taskId === task.id)}
            <div class="task-sessions">{#each $agentWorkspace.sessions.filter((ref) => ref.taskId === task.id) as ref}<span class="task-session" class:live={!!liveFor(ref)}>{ref.kind} · {sessionName(ref)} · {$t(`agentWorkspace.${liveFor(ref) ? 'connected' : 'offline'}`)}</span>{/each}</div>
          {/if}
          <div class="actions"><button disabled={busy || $agentWorkspace.projects.find((p) => p.id === task.projectId)?.archived} on:click={() => editTask(task)}>{$t('agentWorkspace.editTask')}</button><button disabled={busy || task.archived || $agentWorkspace.projects.find((p) => p.id === task.projectId)?.archived} on:click={() => startLink(task)}>{$t('agentWorkspace.linkSession')}</button></div>
        </article>
      {:else}<div class="empty-state"><h2>{$t('agentWorkspace.noTasks')}</h2><p>{$t('agentWorkspace.noTasksHint')}</p></div>{/each}
    {:else}
      <p class="view-hint">{$t('agentWorkspace.sessionHint')}</p>
      {#each sessions as ref (ref.id)}
        {@const live = liveFor(ref)}
        <article class="card" data-testid="workspace-session">
          <div class="card-top"><span class="badge" class:badge-live={!!live}>{ref.kind} · {$t(`agentWorkspace.${live ? 'connected' : 'offline'}`)}</span><small>{projectName(ref.projectId)}</small></div>
          <h3>{sessionName(ref)}</h3>
          <p>{taskName(ref.taskId) || $t('agentWorkspace.projectOnly')}</p>
          <small class="session-path">{ref.host} · {ref.cwd}<br />{ref.file}</small>
          <div class="actions">{#if live}<button on:click={() => openLive(live)}>{$t('agentWorkspace.openTerminal')}</button>{/if}<button disabled={busy} on:click={() => mutate(() => unlinkWorkspaceSession(ref.id))}>{$t('agentWorkspace.unlink')}</button></div>
        </article>
      {:else}<div class="empty-state"><h2>{$t('agentWorkspace.noSessions')}</h2><p>{$t('agentWorkspace.sessionHint')}</p></div>{/each}
    {/if}
  </div>
</section>

<style>
  .workspace { height: 100%; overflow: auto; padding: 24px 32px 40px; box-sizing: border-box; color: var(--text-primary); max-width: 1040px; margin: 0 auto; font-size: 14px; line-height: 1.5; }
  .workspace-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 24px; margin-bottom: 20px; }
  .workspace-header-actions { display: flex; align-items: center; gap: 14px; flex-shrink: 0; }
  h1 { font-size: 22px; margin: 0 0 4px; font-weight: 700; }
  h2 { font-size: 16px; margin: 0; font-weight: 600; }
  h3 { font-size: 15px; margin: 10px 0 6px; font-weight: 600; line-height: 1.35; }
  p { color: var(--text-secondary); margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
  small { color: var(--text-secondary); font-size: 12px; overflow-wrap: anywhere; }
  .status { margin-bottom: 12px; }
  button, input, select, textarea { font: inherit; color: var(--text-primary); background: var(--bg-raised); border: 1px solid var(--border-dim); border-radius: var(--radius-md); padding: 7px 12px; }
  button { cursor: pointer; line-height: 1.3; }
  button:hover:not(:disabled) { border-color: var(--accent); }
  button:disabled { opacity: .45; cursor: default; }
  button:focus-visible, input:focus-visible, select:focus-visible, textarea:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .primary, .chosen { color: var(--accent); background: var(--accent-glow); border-color: var(--border-accent, var(--accent)); }

  /* Projects: one row of chips instead of a column that eats the width. */
  .project-row { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin-bottom: 14px; }
  .project-button { padding: 5px 12px; border-radius: 999px; font-size: 13px; max-width: 320px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .project-button.muted { opacity: .6; }
  .project-button small { color: inherit; opacity: .75; font-size: 12px; }
  .project-add { border-style: dashed; color: var(--text-secondary); }
  .empty { font-size: 13px; margin-bottom: 14px; }
  .project-summary { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; padding: 14px 18px; margin-bottom: 18px; background: var(--bg-surface); border: 1px solid var(--border-dim); border-radius: var(--radius-lg); }
  .project-summary-text { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
  .project-summary p { max-height: 120px; overflow: auto; }

  .workspace-tabs { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; margin: 4px 0 16px; }
  .workspace-tabs span { margin-left: 6px; opacity: .7; }
  .tab-action { margin-left: auto; }
  .view-hint { font-size: 13px; margin-bottom: 14px; }
  .workspace-body { min-width: 0; }

  .card, .editor { background: var(--bg-surface); border: 1px solid var(--border-dim); border-radius: var(--radius-lg); padding: 16px 20px; margin-bottom: 12px; }
  .editor { max-width: 720px; margin-bottom: 20px; }
  .card p { max-height: 200px; overflow: auto; font-size: 13.5px; }
  .card-top { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .urgent { border-left: 3px solid var(--warning); }
  .badge { display: inline-block; font-size: 11.5px; font-weight: 600; letter-spacing: .02em; padding: 2px 9px; border-radius: 999px; color: var(--accent); background: var(--accent-glow); }
  .badge-permission, .badge-blocked { color: #ff7b72; background: rgba(255, 123, 114, 0.14); }
  .badge-done, .badge-live, .badge-result { color: #7ee787; background: rgba(126, 231, 135, 0.14); }
  .badge-in_progress, .badge-input { color: var(--warning); background: rgba(227, 179, 65, 0.14); }
  .badge-review { color: #d2a8ff; background: rgba(210, 168, 255, 0.14); }
  .actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
  .empty-state { padding: 32px 20px; border: 1px dashed var(--border-dim); border-radius: var(--radius-lg); }
  .empty-state h2 { margin-bottom: 6px; }
  .error { background: var(--error-bg); color: var(--error); padding: 12px 14px; border-radius: var(--radius-md); overflow-wrap: anywhere; margin-bottom: 14px; }
  fieldset { border: 0; margin: 12px 0 0; padding: 0; min-width: 0; }
  fieldset label { display: flex; flex-direction: column; gap: 6px; margin-bottom: 12px; font-size: 13px; color: var(--text-secondary); }
  .form-hint { font-size: 13px; margin-bottom: 10px; }
  .archive-toggle { display: flex; align-items: center; gap: 8px; font-size: 12.5px; color: var(--text-secondary); }
  fieldset .archive-toggle { flex-direction: row; }
  .archive-toggle input { width: auto; margin: 0; }
  input, select, textarea { box-sizing: border-box; width: 100%; }
  textarea { resize: vertical; }
  .form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  .form-row label { min-width: 0; }
  .session-path { display: block; font-family: var(--font-mono, monospace); font-size: 11.5px; line-height: 1.6; margin-top: 6px; }
  .task-sessions { display: flex; flex-direction: column; gap: 4px; font-size: 12.5px; color: var(--text-secondary); overflow-wrap: anywhere; margin-top: 6px; }
  .task-session.live { color: #7ee787; }
  @media (max-width: 700px) { .workspace { padding: 16px; } .form-row { grid-template-columns: 1fr; } .workspace-header { flex-direction: column; } }
</style>
