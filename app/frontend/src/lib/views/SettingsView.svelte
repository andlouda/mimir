<script>
  // Settings landing view (presentational). Card actions and folder/update
  // logic stay in the parent and are passed as callbacks. Strings via i18n;
  // shared styles come from the global stylesheets (styles/).
  import { t, locale, availableLocales } from '../i18n.js';
  import { agentDetectionEnabled, agentNotificationsEnabled } from '../stores/agentStore.js';
  import { TERMINAL_FONT_MAX, TERMINAL_FONT_MIN, promptMode, resetTerminalFontSize, terminalFontSize, terminalRenderer, panePlacement, tmuxIntegrationMode, tmuxScrollbackRefill, zoomTerminalFont } from '../stores/uiStore.js';
  import { refreshTmuxStatuses } from '../actions/terminalActions.js';
  import { loadClaudeHookHosts, setClaudeHookInstalledOnHost } from '../actions/agentActions.js';
  import { onMount } from 'svelte';

  // One row per host the hook can live on: this machine and, on Windows,
  // the WSL distro (Claude Code there reads its own settings.json).
  // Linux webview GPU policy (read at start-up, needs a restart).
  let gpuPolicy = '';
  let gpuSaved = '';
  let isLinux = false;
  onMount(async () => {
    try {
      const env = await window['runtime']?.['Environment']?.();
      isLinux = env?.platform === 'linux';
    } catch { isLinux = false; }
    try { gpuPolicy = gpuSaved = await window['go']['main']['App']['GetGPUPolicy'](); } catch { gpuPolicy = gpuSaved = 'never'; }
    try { waylandFix = waylandSaved = await window['go']['main']['App']['GetWaylandFix'](); } catch { waylandFix = waylandSaved = 'auto'; }
    try { isWayland = !!(await window['go']['main']['App']['IsWaylandSession']()); } catch { isWayland = false; }
    try { imModule = imSaved = await window['go']['main']['App']['GetIMModule'](); imEffective = !!(await window['go']['main']['App']['IMModuleEffective']()); } catch { imModule = imSaved = 'auto'; }
  });
  let waylandFix = '';
  let waylandSaved = '';
  let isWayland = false;
  let imModule = '';
  let imSaved = '';
  let imEffective = false;
  async function changePromptMode(event) {
    try { promptMode.set(await window['go']['main']['App']['SetPromptMode'](event.target.value)); } catch (error) { console.error('Could not save prompt mode:', error); }
  }
  async function changeIMModule(event) {
    try {
      imModule = await window['go']['main']['App']['SetIMModule'](event.target.value);
      imEffective = !!(await window['go']['main']['App']['IMModuleEffective']());
    } catch (error) { console.error('Could not save input-method setting:', error); }
  }
  async function changeWaylandFix(event) {
    try {
      waylandFix = await window['go']['main']['App']['SetWaylandFix'](event.target.value);
    } catch (error) {
      console.error('Could not save Wayland setting:', error);
    }
  }
  async function changeGPUPolicy(event) {
    try {
      gpuPolicy = await window['go']['main']['App']['SetGPUPolicy'](event.target.value);
    } catch (error) {
      console.error('Could not save GPU policy:', error);
    }
  }

  // Card descriptions are folded away; a click on the title opens them.
  let openCards = new Set();
  function toggleCard(key) {
    if (openCards.has(key)) openCards.delete(key); else openCards.add(key);
    openCards = openCards;
  }
  let hookHosts = null;
  let hookBusy = '';
  onMount(async () => {
    hookHosts = await loadClaudeHookHosts();
  });
  async function toggleHook(row) {
    hookBusy = row.host;
    try {
      // Outdated installs are refreshed in place (install is idempotent).
      const status = await setClaudeHookInstalledOnHost(row.host, !row.installed || row.outdated);
      hookHosts = hookHosts.map((h) => (h.host === row.host ? { host: row.host, ...status } : h));
    } catch (e) {
      hookHosts = hookHosts.map((h) => (h.host === row.host ? { ...h, error: String(e?.message || e) } : h));
    } finally {
      hookBusy = '';
    }
  }

  async function changeTmuxMode(event) {
    const mode = event.target.value;
    try {
      const saved = await window['go']['main']['App']['SetTmuxIntegrationMode'](mode);
      tmuxIntegrationMode.set(saved || mode);
      // The backend switched running sessions live; pick up their new state.
      await refreshTmuxStatuses();
    } catch (error) {
      console.error('Could not save tmux integration mode:', error);
    }
  }

  export let notesPanelOpen = false;
  export let showFolderManager = false;     // bind
  export let historyTrackingEnabled = false;
  export let aggAvailable = false;
  export let aggStatus = 'missing';
  export let updateChecking = false;
  export let updateInfo = null;
  export let customFolders = [];
  export let newFolderName = '';            // bind
  export let onOpenAISettings = () => {};
  export let onManageTemplates = () => {};
  export let onToggleNotes = () => {};
  export let onToggleHistory = () => {};
  export let onInstallAgg = () => {};
  export let onCheckUpdates = () => {};
  export let onOpenUpdatePage = () => {};
  export let onDownloadUpdate = () => {};
  export let updateDownloading = false;
  export let updateProgress = null;
  export let updateInstalled = false;
  export let onRestartApp = () => {};
  export let onCreateFolder = () => {};
  export let onRenameFolder = () => {};
  export let onDeleteFolder = () => {};
</script>

<div class="ai-hub">
  <div class="ai-hub-header">
    <div>
      <h2>{$t('settings.title')}</h2>
      <p>{$t('settings.subtitle')}</p>
    </div>
    <label class="settings-language">
      <span>{$t('settings.language')}</span>
      <select bind:value={$locale}>
        {#each availableLocales as loc (loc)}
          <option value={loc}>{$t(`settings.languages.${loc}`)}</option>
        {/each}
      </select>
    </label>
  </div>

  <div class="ai-hub-grid">
    <div class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">A</span>
        <span class="settings-stepper" role="group" aria-label={$t('settings.cards.fontSize.title')}>
          <button type="button" class="settings-inline-btn" on:click={() => zoomTerminalFont(-1)} disabled={$terminalFontSize <= TERMINAL_FONT_MIN} aria-label={$t('settings.cards.fontSize.smaller')}>−</button>
          <span class="settings-stepper-value">{$terminalFontSize}px</span>
          <button type="button" class="settings-inline-btn" on:click={() => zoomTerminalFont(1)} disabled={$terminalFontSize >= TERMINAL_FONT_MAX} aria-label={$t('settings.cards.fontSize.larger')}>+</button>
          <button type="button" class="settings-inline-btn" on:click={resetTerminalFontSize}>{$t('settings.cards.fontSize.reset')}</button>
        </span>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('fontSize')} on:click|preventDefault|stopPropagation={() => toggleCard('fontSize')}><strong>{$t('settings.cards.fontSize.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('fontSize') ? '▾' : '▸'}</span></button>
      {#if openCards.has('fontSize')}
      <p>{$t('settings.cards.fontSize.desc')}</p>
      {/if}
    </div>
    <label class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x25A3;</span>
        <select bind:value={$terminalRenderer}>
          <option value="auto">{$t('settings.cards.renderer.auto')}</option>
          <option value="dom">{$t('settings.cards.renderer.dom')}</option>
        </select>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('renderer')} on:click|preventDefault|stopPropagation={() => toggleCard('renderer')}><strong>{$t('settings.cards.renderer.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('renderer') ? '▾' : '▸'}</span></button>
      {#if openCards.has('renderer')}
      <p>{$t(`settings.cards.renderer.desc_${$terminalRenderer}`)}</p>
      {/if}
    </label>
    {#if isLinux}
      <label class="ai-hub-card settings-toggle-card">
        <div class="ai-hub-card-top">
          <span class="ai-hub-icon">&#x2699;</span>
          <select value={gpuPolicy} on:change={changeGPUPolicy}>
            <option value="never">{$t('settings.cards.gpu.never')}</option>
            <option value="ondemand">{$t('settings.cards.gpu.ondemand')}</option>
            <option value="always">{$t('settings.cards.gpu.always')}</option>
          </select>
        </div>
        <button type="button" class="settings-card-title" aria-expanded={openCards.has('gpu')} on:click|preventDefault|stopPropagation={() => toggleCard('gpu')}><strong>{$t('settings.cards.gpu.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('gpu') ? '▾' : '▸'}</span></button>
        {#if openCards.has('gpu')}
        <p>{$t('settings.cards.gpu.desc')}</p>
        {/if}
        {#if gpuPolicy !== gpuSaved}<p class="settings-note">{$t('settings.cards.gpu.restart')}</p>{/if}
      </label>
      <label class="ai-hub-card settings-toggle-card">
        <div class="ai-hub-card-top">
          <span class="ai-hub-icon">&#x25A6;</span>
          <select value={waylandFix} on:change={changeWaylandFix}>
            <option value="auto">{$t('settings.cards.wayland.auto')}</option>
            <option value="on">{$t('settings.cards.wayland.on')}</option>
            <option value="off">{$t('settings.cards.wayland.off')}</option>
          </select>
        </div>
        <button type="button" class="settings-card-title" aria-expanded={openCards.has('wayland')} on:click|preventDefault|stopPropagation={() => toggleCard('wayland')}><strong>{$t('settings.cards.wayland.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('wayland') ? '▾' : '▸'}</span></button>
        {#if openCards.has('wayland')}
        <p>{$t('settings.cards.wayland.desc')}{isWayland ? ' ' + $t('settings.cards.wayland.detected') : ''}</p>
        {/if}
        {#if waylandFix !== waylandSaved}<p class="settings-note">{$t('settings.cards.gpu.restart')}</p>{/if}
      </label>
      <label class="ai-hub-card settings-toggle-card">
        <div class="ai-hub-card-top">
          <span class="ai-hub-icon">&#x00E4;</span>
          <select value={imModule} on:change={changeIMModule}>
            <option value="auto">{$t('settings.cards.im.auto')}</option>
            <option value="simple">{$t('settings.cards.im.simple')}</option>
            <option value="system">{$t('settings.cards.im.system')}</option>
          </select>
        </div>
        <button type="button" class="settings-card-title" aria-expanded={openCards.has('im')} on:click|preventDefault|stopPropagation={() => toggleCard('im')}><strong>{$t('settings.cards.im.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('im') ? '▾' : '▸'}</span></button>
        {#if openCards.has('im')}
        <p>{$t('settings.cards.im.desc')}</p>
        <p class="settings-note">{imEffective ? $t('settings.cards.im.effectiveOn') : $t('settings.cards.im.effectiveOff')}{imModule !== imSaved ? ' ' + $t('settings.cards.gpu.restart') : ''}</p>
        {/if}
      </label>
    {/if}
    <label class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x276F;</span>
        <select value={$promptMode} on:change={changePromptMode}>
          <option value="mimir">{$t('settings.cards.prompt.mimir')}</option>
          <option value="shell">{$t('settings.cards.prompt.shell')}</option>
        </select>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('prompt')} on:click|preventDefault|stopPropagation={() => toggleCard('prompt')}><strong>{$t('settings.cards.prompt.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('prompt') ? '▾' : '▸'}</span></button>
      {#if openCards.has('prompt')}
      <p>{$t(`settings.cards.prompt.desc_${$promptMode}`)}</p>
      <p class="settings-note">{$t('settings.cards.prompt.note')}</p>
      {/if}
    </label>
    <label class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x2261;</span>
        <select value={$tmuxIntegrationMode} on:change={changeTmuxMode}>
          <option value="invisible">{$t('settings.cards.tmuxMode.invisible')}</option>
          <option value="classic">{$t('settings.cards.tmuxMode.classic')}</option>
          <option value="off">{$t('settings.cards.tmuxMode.off')}</option>
        </select>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('tmuxMode')} on:click|preventDefault|stopPropagation={() => toggleCard('tmuxMode')}><strong>{$t('settings.cards.tmuxMode.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('tmuxMode') ? '▾' : '▸'}</span></button>
      {#if openCards.has('tmuxMode')}
      <p>{$t(`settings.cards.tmuxMode.desc_${$tmuxIntegrationMode}`)}</p>
      <p class="settings-note">{$t('settings.cards.tmuxMode.note')}</p>
      {/if}
    </label>
    <label class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x21A9;</span>
        <input type="checkbox" bind:checked={$tmuxScrollbackRefill} />
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('tmuxRefill')} on:click|preventDefault|stopPropagation={() => toggleCard('tmuxRefill')}><strong>{$t('settings.cards.tmuxRefill.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('tmuxRefill') ? '▾' : '▸'}</span></button>
      {#if openCards.has('tmuxRefill')}
      <p>{$t('settings.cards.tmuxRefill.desc')}</p>
      {/if}
    </label>
    <label class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x25EB;</span>
        <select bind:value={$panePlacement}>
          <option value="row">{$t('settings.cards.panePlacement.row')}</option>
          <option value="column">{$t('settings.cards.panePlacement.column')}</option>
          <option value="auto">{$t('settings.cards.panePlacement.auto')}</option>
        </select>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('panePlacement')} on:click|preventDefault|stopPropagation={() => toggleCard('panePlacement')}><strong>{$t('settings.cards.panePlacement.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('panePlacement') ? '▾' : '▸'}</span></button>
      {#if openCards.has('panePlacement')}
      <p>{$t('settings.cards.panePlacement.desc')}</p>
      {/if}
    </label>
    <label class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x2731;</span>
        <input type="checkbox" bind:checked={$agentDetectionEnabled} />
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('agentDetection')} on:click|preventDefault|stopPropagation={() => toggleCard('agentDetection')}><strong>{$t('settings.cards.agentDetection.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('agentDetection') ? '▾' : '▸'}</span></button>
      {#if openCards.has('agentDetection')}
      <p>{$t('settings.cards.agentDetection.desc')}</p>
      {/if}
    </label>
    <label class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x266A;</span>
        <input type="checkbox" bind:checked={$agentNotificationsEnabled} />
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('agentNotify')} on:click|preventDefault|stopPropagation={() => toggleCard('agentNotify')}><strong>{$t('settings.cards.agentNotify.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('agentNotify') ? '▾' : '▸'}</span></button>
      {#if openCards.has('agentNotify')}
      <p>{$t('settings.cards.agentNotify.desc')}</p>
      {/if}
    </label>
    <div class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x2731;</span>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('claudeHook')} on:click|preventDefault|stopPropagation={() => toggleCard('claudeHook')}><strong>{$t('settings.cards.claudeHook.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('claudeHook') ? '▾' : '▸'}</span></button>
      {#if openCards.has('claudeHook')}
      <p>{$t('settings.cards.claudeHook.desc')}</p>
      {/if}
      {#if hookHosts}
        <ul class="settings-host-list">
          {#each hookHosts as row (row.host)}
            <li class="settings-host-row">
              <span class="settings-host-name">{$t(`settings.cards.claudeHook.host_${row.host}`)}</span>
              <span class="settings-host-status" class:settings-host-ok={row.installed && !row.outdated} class:settings-host-err={!!row.error} title={row.settingsPath || ''}>
                {row.error ? row.error : row.outdated ? $t('settings.cards.claudeHook.outdated') : row.installed ? $t('settings.cards.claudeHook.installed') : $t('settings.cards.claudeHook.notInstalled')}
              </span>
              <button type="button" class="settings-inline-btn" on:click={() => toggleHook(row)} disabled={hookBusy !== '' || !!row.error}>
                {hookBusy === row.host ? '…' : row.outdated ? $t('settings.cards.claudeHook.update') : row.installed ? $t('settings.cards.claudeHook.remove') : $t('settings.cards.claudeHook.install')}
              </button>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="settings-note">…</p>
      {/if}
      {#if openCards.has('claudeHook')}<p class="settings-note">{$t('settings.cards.claudeHook.note')}</p>{/if}
    </div>
    <div class="ai-hub-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x269B;</span>
        <button type="button" class="ai-hub-link" on:click={onOpenAISettings}>{$t('settings.actions.configure')}</button>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('aiSettings')} on:click|preventDefault|stopPropagation={() => toggleCard('aiSettings')}><strong>{$t('settings.cards.aiSettings.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('aiSettings') ? '▾' : '▸'}</span></button>
      {#if openCards.has('aiSettings')}
      <p>{$t('settings.cards.aiSettings.desc')}</p>
      {/if}
    </div>

    <div class="ai-hub-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#9998;</span>
        <button type="button" class="ai-hub-link" on:click={onManageTemplates}>{$t('settings.actions.manage')}</button>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('templates')} on:click|preventDefault|stopPropagation={() => toggleCard('templates')}><strong>{$t('settings.cards.templates.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('templates') ? '▾' : '▸'}</span></button>
      {#if openCards.has('templates')}
      <p>{$t('settings.cards.templates.desc')}</p>
      {/if}
    </div>

    <div class="ai-hub-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x270E;</span>
        <button type="button" class="ai-hub-link" on:click={onToggleNotes}>{notesPanelOpen ? $t('settings.actions.close') : $t('settings.actions.open')}</button>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('notes')} on:click|preventDefault|stopPropagation={() => toggleCard('notes')}><strong>{$t('settings.cards.notes.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('notes') ? '▾' : '▸'}</span></button>
      {#if openCards.has('notes')}
      <p>{$t('settings.cards.notes.desc')}</p>
      {/if}
    </div>

    <div class="ai-hub-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x2302;</span>
        <button type="button" class="ai-hub-link" on:click={() => { showFolderManager = !showFolderManager; }}>{showFolderManager ? $t('settings.actions.close') : $t('settings.actions.manage')}</button>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('folders')} on:click|preventDefault|stopPropagation={() => toggleCard('folders')}><strong>{$t('settings.cards.folders.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('folders') ? '▾' : '▸'}</span></button>
      {#if openCards.has('folders')}
      <p>{$t('settings.cards.folders.desc')}</p>
      {/if}
    </div>

    <div class="ai-hub-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x2261;</span>
        <button type="button" class="ai-hub-link" on:click={onToggleHistory}>{historyTrackingEnabled ? $t('settings.actions.enabled') : $t('settings.actions.disabled')}</button>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('history')} on:click|preventDefault|stopPropagation={() => toggleCard('history')}><strong>{$t('settings.cards.history.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('history') ? '▾' : '▸'}</span></button>
      {#if openCards.has('history')}
      <p>{historyTrackingEnabled ? $t('settings.cards.history.enabledDesc') : $t('settings.cards.history.disabledDesc')}</p>
      {/if}
    </div>

    <div class="ai-hub-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x25B8;</span>
        <button type="button" class="ai-hub-link" on:click={onInstallAgg} disabled={aggAvailable}>{aggAvailable ? $t('settings.actions.installed') : $t('settings.actions.install')}</button>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('agg')} on:click|preventDefault|stopPropagation={() => toggleCard('agg')}><strong>{$t('settings.cards.agg.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('agg') ? '▾' : '▸'}</span></button>
      {#if openCards.has('agg')}
      <p>{#if aggAvailable}{$t('settings.cards.agg.installedDesc')}{:else if aggStatus === 'incompatible'}{$t('settings.cards.agg.incompatibleDesc')}{:else}{$t('settings.cards.agg.missingDesc')}{/if}</p>
      {/if}
    </div>

    <div class="ai-hub-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x21E7;</span>
        <button type="button" class="ai-hub-link" on:click={onCheckUpdates} disabled={updateChecking}>{updateChecking ? $t('settings.actions.checking') : (updateInfo?.updateAvailable ? $t('settings.actions.available') : $t('settings.actions.check'))}</button>
      </div>
      <button type="button" class="settings-card-title" aria-expanded={openCards.has('updates')} on:click|preventDefault|stopPropagation={() => toggleCard('updates')}><strong>{$t('settings.cards.updates.title')}</strong><span class="settings-card-chevron" aria-hidden="true">{openCards.has('updates') ? '▾' : '▸'}</span></button>
      {#if openCards.has('updates')}
      <p>
        {#if updateInfo?.error}
          {updateInfo.error}
        {:else if updateInstalled}
          {$t('settings.cards.updates.pendingDesc', { version: updateInfo?.latestVersion || '?' })}
        {:else if updateInfo?.updateAvailable}
          {$t('settings.cards.updates.availableDesc', { version: updateInfo.latestVersion })}
        {:else if updateInfo}
          {$t('settings.cards.updates.currentDesc', { version: updateInfo.currentVersion })}
        {:else}
          {$t('settings.cards.updates.defaultDesc')}
        {/if}
      </p>
      {/if}
    </div>
  </div>

  <div class="shortcuts-panel">
    <h3>{$t('settings.shortcuts.title')}</h3>
    <div class="shortcuts-grid">
      <div class="shortcut-row">
        <kbd>Ctrl</kbd> + <kbd>Shift</kbd> + <kbd>P</kbd>
        <span>{$t('settings.shortcuts.templatePicker')}</span>
      </div>
      <div class="shortcut-row">
        <kbd>Ctrl</kbd> + <kbd>Shift</kbd> + <kbd>W</kbd>
        <span>{$t('settings.shortcuts.workflowPicker')}</span>
      </div>
      <div class="shortcut-row">
        <kbd>Ctrl</kbd> + <kbd>Shift</kbd> + <kbd>F</kbd>
        <span>{$t('settings.shortcuts.terminalSearch')}</span>
      </div>
      <div class="shortcut-row">
        <kbd>Ctrl</kbd> + <kbd>Shift</kbd> + <kbd>N</kbd>
        <span>{$t('settings.shortcuts.notesPanel')}</span>
      </div>
    </div>
  </div>

  {#if showFolderManager}
    <div class="folder-manager">
      <h3>{$t('settings.folderManager.title')}</h3>
      <ul class="folder-manager-list">
        {#each customFolders as f (f.id)}
          <li class="folder-manager-item">
            <input
              type="text"
              class="folder-manager-input"
              value={f.name}
              on:blur={(e) => onRenameFolder(f, e.target.value)}
              on:keydown={(e) => { if (e.key === 'Enter') e.target.blur(); }}
            />
            <button type="button" class="folder-manager-delete" on:click={() => onDeleteFolder(f.id)} title={$t('settings.folderManager.delete')}>&#x2715;</button>
          </li>
        {/each}
      </ul>
      <div class="folder-manager-add">
        <input
          type="text"
          class="folder-manager-input"
          placeholder={$t('settings.folderManager.newPlaceholder')}
          bind:value={newFolderName}
          on:keydown={(e) => { if (e.key === 'Enter') onCreateFolder(); }}
        />
        <button type="button" class="folder-manager-add-btn" on:click={onCreateFolder} disabled={!newFolderName.trim()}>+</button>
      </div>
    </div>
  {/if}

  {#if updateInfo}
    <div class="settings-inline-panel">
      <div>
        <strong>{$t('settings.updatePanel.status')}</strong>
        <p>
          {$t('settings.updatePanel.current')}: {updateInfo.currentVersion || 'unknown'}
          {#if updateInfo.latestVersion}
            · {$t('settings.updatePanel.latest')}: {updateInfo.latestVersion}
          {/if}
          {#if updateInfo.platform}
            · {updateInfo.platform}
          {/if}
        </p>
        {#if updateInfo.platformAsset}
          <p>{$t('settings.updatePanel.asset')}: {updateInfo.platformAsset.name}</p>
        {/if}
        {#if updateInfo.expectedSHA256}
          <p>SHA256: <code class="sha256-hash">{updateInfo.expectedSHA256}</code></p>
        {:else if updateInfo.checksumAsset}
          <p>{$t('settings.updatePanel.checksums')}: {updateInfo.checksumAsset.name}</p>
        {/if}
        {#if updateInfo.executablePath}
          <p>{$t('settings.updatePanel.executable')}: <code>{updateInfo.executablePath}</code></p>
        {/if}
        {#if !updateInfo.configured}
          <p>{$t('settings.updatePanel.notConfigured')}</p>
        {/if}
      </div>
      <div class="settings-inline-actions">
        <button type="button" class="modal-secondary-button" on:click={onCheckUpdates} disabled={updateChecking || updateDownloading}>{$t('settings.updatePanel.refresh')}</button>
        {#if updateInstalled}
          <span class="update-staged-msg">{$t('settings.updatePanel.restartRequired')}</span>
          <button type="button" class="modal-primary-button" on:click={onRestartApp}>{$t('settings.updatePanel.restartNow')}</button>
        {:else if updateDownloading}
          <div class="update-progress-inline">
            <span class="update-progress-label">{$t(`settings.updatePanel.stage_${updateProgress?.stage || 'downloading'}`)}</span>
            {#if updateProgress?.percent >= 0}
              <progress value={updateProgress.percent} max="100"></progress>
            {/if}
          </div>
        {:else if updateInfo?.updateAvailable && !updateInfo?.manualUpdateOnly}
          <button type="button" class="modal-primary-button" on:click={onDownloadUpdate}>{$t('settings.updatePanel.downloadInstall')}</button>
        {/if}
        <button type="button" class="modal-secondary-button" on:click={onOpenUpdatePage} disabled={!updateInfo.configured}>{$t('settings.updatePanel.openRelease')}</button>
      </div>
    </div>
  {/if}
</div>

<style>
  .settings-card-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    width: 100%;
    background: none;
    border: 0;
    padding: 0;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }
  .settings-card-title strong { font-weight: 600; }
  .settings-card-chevron { color: var(--text-muted); font-size: 0.8rem; flex-shrink: 0; }
  .ai-hub-card p { margin-top: 0.55rem; }
  /* An opened description must not stretch the neighbours in the row. */
  :global(.ai-hub-grid) { align-items: start; }

  .sha256-hash {
    font-size: 0.7rem;
    word-break: break-all;
    user-select: all;
    opacity: 0.8;
  }
  .update-staged-msg {
    color: var(--accent);
    font-size: 0.78rem;
    font-weight: 600;
  }
  .update-progress-inline {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.78rem;
    color: var(--text-secondary);
  }
  .update-progress-inline progress {
    width: 120px;
    height: 6px;
    appearance: none;
    border: none;
    border-radius: 3px;
    overflow: hidden;
    background: var(--bg-void);
  }
  .update-progress-inline progress::-webkit-progress-bar {
    background: var(--bg-void);
    border-radius: 3px;
  }
  .update-progress-inline progress::-webkit-progress-value {
    background: var(--accent);
    border-radius: 3px;
  }
  .update-progress-label {
    white-space: nowrap;
  }
  .settings-language {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.78rem;
    color: var(--text-secondary);
  }
  .settings-language select {
    background: var(--bg-surface);
    color: var(--text-primary);
    border: 1px solid var(--border-dim);
    border-radius: var(--radius-sm);
    padding: 0.35rem 0.5rem;
    font-family: var(--font-sans);
    font-size: 0.78rem;
  }
  .shortcuts-panel {
    margin-top: 1.2rem;
    border: 1px solid var(--border-dim);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
    padding: 1rem 1.2rem;
  }
  .shortcuts-panel h3 {
    margin: 0 0 0.75rem;
    font-size: 0.88rem;
    color: var(--text-primary);
  }
  .shortcuts-grid {
    display: grid;
    gap: 0.5rem;
  }
  .shortcut-row {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    font-size: 0.8rem;
    color: var(--text-secondary);
  }
  .shortcut-row span {
    margin-left: 0.6rem;
  }
  .shortcut-row kbd {
    display: inline-block;
    padding: 0.15rem 0.45rem;
    background: var(--bg-overlay);
    border: 1px solid var(--border-dim);
    border-radius: var(--radius-sm);
    font-family: var(--font-mono, monospace);
    font-size: 0.72rem;
    color: var(--text-primary);
    line-height: 1.4;
    box-shadow: 0 1px 0 var(--border-dim);
  }
</style>
