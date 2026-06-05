<script>
  // Left navigation sidebar. Collapse/disclosure/drag state is local to this
  // component; data and actions come from the parent via props/callbacks.
  import { t } from './i18n.js';

  export let currentPage = 'terminals';
  export let terminals = [];
  export let sshProfiles = [];
  export let customFolders = [];
  export let groups = [];                 // grouped sidebar terminals
  export let foldersOpen = {};            // terminalSessionFoldersOpen map
  export let activeTerminalId = null;
  export let openPage = () => {};
  export let openTerminals = () => {};    // clears edit state + opens terminals page
  export let openSSHProfilePicker = () => {};
  export let refreshSSHProfiles = () => {};
  export let isAuditPage = () => false;
  export let assignTerminalToFolder = () => {};
  export let toggleTerminalFolder = () => {};
  export let selectTerminal = () => {};
  export let connectSSHProfile = () => {};
  export let onResize = () => {};

  // Sidebar-local UI state (not shared with the rest of the app).
  let collapsed = false;
  let terminalNavOpen = false;
  let sshNavOpen = true;
  let auditNavOpen = false;
  let dragOverFolder = null;
  let dragTerminalId = null;

  function toggleSSHNav() {
    sshNavOpen = !sshNavOpen;
    if (sshNavOpen) {
      refreshSSHProfiles();
    }
  }
</script>

<nav class="sidebar" class:collapsed={collapsed}>
  <div class="sidebar-brand">
    <button class="sidebar-toggle" on:click={() => { collapsed = !collapsed; setTimeout(onResize, 220); }} title={collapsed ? $t('sidebar.expand') : $t('sidebar.collapse')}>
      {collapsed ? '▶' : '◀'}
    </button>
    {#if !collapsed}
      <span class="brand-icon">&#x16C7;</span>
      <span class="brand-text">Mimir</span>
    {/if}
  </div>

  {#if collapsed}
    <div class="sidebar-section">
      <div class="sidebar-heading collapsed-icon" on:click={openTerminals} on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') openTerminals(); }} tabindex="0" role="button" class:active-nav={currentPage === 'terminals'} title={$t('sidebar.terminal')}>
        <span class="nav-icon">&#9656;</span>
      </div>
    </div>
    <div class="sidebar-section">
      <div class="sidebar-heading collapsed-icon" on:click={openSSHProfilePicker} on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') openSSHProfilePicker(); }} tabindex="0" role="button" title={$t('sidebar.sshHosts')}>
        <span class="nav-icon">&#x2192;</span>
      </div>
    </div>
    <div class="sidebar-section">
      <div class="sidebar-heading collapsed-icon" on:click={() => { openPage("fileBrowser"); }} on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { openPage("fileBrowser"); }}} tabindex="0" role="button" class:active-nav={currentPage === 'fileBrowser'} title={$t('sidebar.files')}>
        <span class="nav-icon">&#x2302;</span>
      </div>
    </div>
    <div class="sidebar-section">
      <div class="sidebar-heading collapsed-icon" on:click={() => { openPage("workflowBuilder"); }} on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { openPage("workflowBuilder"); }}} tabindex="0" role="button" class:active-nav={currentPage === 'workflowBuilder'} title={$t('sidebar.workflow')}>
        <span class="nav-icon">&#x2699;</span>
      </div>
    </div>
    <div class="sidebar-section">
      <div class="sidebar-heading collapsed-icon" on:click={() => { openPage("aiHub"); }} on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { openPage("aiHub"); }}} tabindex="0" role="button" class:active-nav={currentPage === 'aiHub'} title={$t('sidebar.ai')}>
        <span class="nav-icon">&#x269B;</span>
      </div>
    </div>
    <div class="sidebar-section">
      <div class="sidebar-heading collapsed-icon" on:click={() => { openPage("activityLogs"); }} on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { openPage("activityLogs"); }}} tabindex="0" role="button" class:active-nav={isAuditPage()} title={$t('sidebar.audit')}>
        <span class="nav-icon">&#x1F4DC;</span>
      </div>
    </div>
    <div class="sidebar-section">
      <div class="sidebar-heading collapsed-icon" on:click={() => { openPage("settings"); }} on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { openPage("settings"); }}} tabindex="0" role="button" class:active-nav={currentPage === 'settings' || currentPage === 'templateManager'} title={$t('sidebar.settings')}>
        <span class="nav-icon">&#9881;</span>
      </div>
    </div>
  {:else}
    <div class="sidebar-section">
      <div class="sidebar-heading"
        on:click={() => { terminalNavOpen = !terminalNavOpen; if (currentPage !== 'terminals') openTerminals(); }}
        on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { terminalNavOpen = !terminalNavOpen; if (currentPage !== 'terminals') openTerminals(); }}}
        tabindex="0" role="button"
        class:active-nav={currentPage === 'terminals'}
        aria-expanded={terminalNavOpen}
      >
        <span class="nav-icon">&#9656;</span> {$t('sidebar.terminal')}
        <span class="sidebar-disclosure">{terminalNavOpen ? '▾' : '▸'}</span>
      </div>
      {#if terminalNavOpen && terminals.length > 0}
        <ul class="sidebar-list sidebar-subnav">
          {#each groups as group (group.id)}
            <li
              class="sidebar-folder"
              class:sidebar-folder-drop-target={group.isCustom && dragOverFolder === group.id}
              on:dragover|preventDefault={(e) => { if (dragTerminalId != null && group.isCustom) { dragOverFolder = group.id; e.dataTransfer.dropEffect = 'move'; } }}
              on:dragleave={() => { if (dragOverFolder === group.id) dragOverFolder = null; }}
              on:drop|preventDefault={() => { if (dragTerminalId != null && group.isCustom) { const fid = group.id.replace('folder:', ''); assignTerminalToFolder(dragTerminalId, fid); dragTerminalId = null; dragOverFolder = null; } }}
            >
              <button class="sidebar-folder-button" on:click={() => toggleTerminalFolder(group.id)}>
                <span>{(foldersOpen[group.id] ?? true) ? '▾' : '▸'} {group.label}</span>
                <small>{group.terminals.length}</small>
              </button>
            </li>
            {#if foldersOpen[group.id] ?? true}
              {#each group.terminals as term (term.id)}
                <li>
                  <button
                    class:active-subnav={activeTerminalId === term.id}
                    title={`${term.name} (${term.type})${term.minimized ? ' - minimized' : ''}`}
                    draggable={customFolders.length > 0}
                    on:click={() => selectTerminal(term)}
                    on:dragstart={(e) => { dragTerminalId = term.id; e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', String(term.id)); }}
                    on:dragend={() => { dragTerminalId = null; dragOverFolder = null; }}
                  >
                    <span class="sidebar-terminal-name">{term.name}</span>
                    {#if term.minimized}<small>{$t('sidebar.minimized')}</small>{/if}
                  </button>
                </li>
              {/each}
            {/if}
          {/each}
        </ul>
        {#if dragTerminalId != null}
          <div
            class="sidebar-unassign-drop"
            class:sidebar-folder-drop-target={dragOverFolder === '__auto__'}
            role="button"
            tabindex="-1"
            aria-label={$t('sidebar.autoGroup')}
            on:dragover|preventDefault={(e) => { dragOverFolder = '__auto__'; e.dataTransfer.dropEffect = 'move'; }}
            on:dragleave={() => { if (dragOverFolder === '__auto__') dragOverFolder = null; }}
            on:drop|preventDefault={() => { if (dragTerminalId != null) { assignTerminalToFolder(dragTerminalId, ''); dragTerminalId = null; dragOverFolder = null; } }}
          >
            {$t('sidebar.autoGroup')}
          </div>
        {/if}
      {/if}
    </div>

    <div class="sidebar-section sidebar-section-compact">
      <div class="sidebar-heading"
        on:click={toggleSSHNav}
        on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { toggleSSHNav(); } }}
        tabindex="0" role="button"
        aria-expanded={sshNavOpen}
      >
        <span class="nav-icon">&#x2192;</span> {$t('sidebar.sshHosts')}
        <button class="sidebar-add-btn" on:click|stopPropagation={() => { openSSHProfilePicker(); }} title={$t('sidebar.manageSSH')}>+</button>
        <span class="sidebar-disclosure">{sshNavOpen ? '▾' : '▸'}</span>
      </div>
      {#if sshNavOpen}
        {#if sshProfiles.length === 0}
          <p class="sidebar-empty">{$t('sidebar.noProfiles')}</p>
        {:else}
          <ul class="sidebar-list sidebar-ssh-list">
            {#each sshProfiles as profile (profile.id)}
              <li>
                <button
                  on:click={() => connectSSHProfile(profile)}
                  on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') connectSSHProfile(profile); }}
                  tabindex="0"
                  title="{profile.username}@{profile.host}:{profile.port}"
                >
                  {profile.name}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
    </div>

    <div class="sidebar-section">
      <div class="sidebar-heading"
        on:click={() => { openPage("fileBrowser"); }}
        on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { openPage("fileBrowser"); }}}
        tabindex="0" role="button"
        class:active-nav={currentPage === 'fileBrowser'}
      >
        <span class="nav-icon">&#x2302;</span> {$t('sidebar.files')}{#if terminals.some(t => t.type === 'ssh' && !t.disconnected)}<span class="ssh-files-dot"></span>{/if}
      </div>
    </div>

    <div class="sidebar-section">
      <div class="sidebar-heading"
        on:click={() => { openPage("workflowBuilder"); }}
        on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { openPage("workflowBuilder"); }}}
        tabindex="0" role="button"
        class:active-nav={currentPage === 'workflowBuilder'}
      >
        <span class="nav-icon">&#x2699;</span> {$t('sidebar.workflow')}
      </div>
    </div>

    <div class="sidebar-section">
      <div class="sidebar-heading"
        on:click={() => { openPage("aiHub"); }}
        on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { openPage("aiHub"); }}}
        tabindex="0" role="button"
        class:active-nav={currentPage === 'aiHub'}
      >
        <span class="nav-icon">&#x269B;</span> {$t('sidebar.ai')}
      </div>
    </div>

    <div class="sidebar-section">
      <div class="sidebar-heading"
        on:click={() => { auditNavOpen = !auditNavOpen; }}
        on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { auditNavOpen = !auditNavOpen; }}}
        tabindex="0" role="button"
        class:active-nav={isAuditPage()}
        aria-expanded={auditNavOpen || isAuditPage()}
      >
        <span class="nav-icon">&#x1F4DC;</span> {$t('sidebar.audit')}
        <span class="sidebar-disclosure">{(auditNavOpen || isAuditPage()) ? '▾' : '▸'}</span>
      </div>
      {#if auditNavOpen || isAuditPage()}
        <ul class="sidebar-list sidebar-subnav">
          <li><button class:active-subnav={currentPage === 'activityLogs'} on:click={() => openPage("activityLogs")}>{$t('sidebar.logs')}</button></li>
          <li><button class:active-subnav={currentPage === 'historyDashboard'} on:click={() => openPage("historyDashboard")}>{$t('sidebar.history')}</button></li>
          <li><button class:active-subnav={currentPage === 'recordings'} on:click={() => openPage("recordings")}>{$t('sidebar.recordings')}</button></li>
        </ul>
      {/if}
    </div>

    <div class="sidebar-section">
      <div class="sidebar-heading"
        on:click={() => { openPage("settings"); }}
        on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { openPage("settings"); }}}
        tabindex="0" role="button"
        class:active-nav={currentPage === 'settings' || currentPage === 'templateManager'}
      >
        <span class="nav-icon">&#9881;</span> {$t('sidebar.settings')}
      </div>
    </div>
  {/if}
</nav>
