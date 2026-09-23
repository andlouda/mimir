<script>
  // Settings landing view (presentational). Card actions and folder/update
  // logic stay in the parent and are passed as callbacks. Strings via i18n;
  // shared styles come from the global stylesheets (styles/).
  import { t, locale, availableLocales } from '../i18n.js';
  import { agentDetectionEnabled } from '../stores/agentStore.js';
  import { tmuxIntegrationMode } from '../stores/uiStore.js';
  import { refreshTmuxStatuses } from '../actions/terminalActions.js';
  import { loadClaudeHookHosts, setClaudeHookInstalledOnHost } from '../actions/agentActions.js';
  import { onMount } from 'svelte';

  // One row per host the hook can live on: this machine and, on Windows,
  // the WSL distro (Claude Code there reads its own settings.json).
  let hookHosts = null;
  let hookBusy = '';
  onMount(async () => {
    hookHosts = await loadClaudeHookHosts();
  });
  async function toggleHook(row) {
    hookBusy = row.host;
    try {
      const status = await setClaudeHookInstalledOnHost(row.host, !row.installed);
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
    <label class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x2261;</span>
        <select value={$tmuxIntegrationMode} on:change={changeTmuxMode}>
          <option value="invisible">{$t('settings.cards.tmuxMode.invisible')}</option>
          <option value="classic">{$t('settings.cards.tmuxMode.classic')}</option>
          <option value="off">{$t('settings.cards.tmuxMode.off')}</option>
        </select>
      </div>
      <strong>{$t('settings.cards.tmuxMode.title')}</strong>
      <p>{$t(`settings.cards.tmuxMode.desc_${$tmuxIntegrationMode}`)}</p>
      <p class="settings-note">{$t('settings.cards.tmuxMode.note')}</p>
    </label>
    <label class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x1F916;</span>
        <input type="checkbox" bind:checked={$agentDetectionEnabled} />
      </div>
      <strong>{$t('settings.cards.agentDetection.title')}</strong>
      <p>{$t('settings.cards.agentDetection.desc')}</p>
    </label>
    <div class="ai-hub-card settings-toggle-card">
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x2731;</span>
      </div>
      <strong>{$t('settings.cards.claudeHook.title')}</strong>
      <p>{$t('settings.cards.claudeHook.desc')}</p>
      {#if hookHosts}
        <ul class="settings-host-list">
          {#each hookHosts as row (row.host)}
            <li class="settings-host-row">
              <span class="settings-host-name">{$t(`settings.cards.claudeHook.host_${row.host}`)}</span>
              <span class="settings-host-status" class:settings-host-ok={row.installed} class:settings-host-err={!!row.error} title={row.settingsPath || ''}>
                {row.error ? row.error : row.installed ? $t('settings.cards.claudeHook.installed') : $t('settings.cards.claudeHook.notInstalled')}
              </span>
              <button type="button" class="settings-inline-btn" on:click={() => toggleHook(row)} disabled={hookBusy !== '' || !!row.error}>
                {hookBusy === row.host ? '…' : row.installed ? $t('settings.cards.claudeHook.remove') : $t('settings.cards.claudeHook.install')}
              </button>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="settings-note">…</p>
      {/if}
      <p class="settings-note">{$t('settings.cards.claudeHook.note')}</p>
    </div>
    <button type="button" class="ai-hub-card" on:click={onOpenAISettings}>
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x269B;</span>
        <span class="ai-hub-link">{$t('settings.actions.configure')}</span>
      </div>
      <strong>{$t('settings.cards.aiSettings.title')}</strong>
      <p>{$t('settings.cards.aiSettings.desc')}</p>
    </button>

    <button type="button" class="ai-hub-card" on:click={onManageTemplates}>
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#9998;</span>
        <span class="ai-hub-link">{$t('settings.actions.manage')}</span>
      </div>
      <strong>{$t('settings.cards.templates.title')}</strong>
      <p>{$t('settings.cards.templates.desc')}</p>
    </button>

    <button type="button" class="ai-hub-card" on:click={onToggleNotes}>
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x1F4DD;</span>
        <span class="ai-hub-link">{notesPanelOpen ? $t('settings.actions.close') : $t('settings.actions.open')}</span>
      </div>
      <strong>{$t('settings.cards.notes.title')}</strong>
      <p>{$t('settings.cards.notes.desc')}</p>
    </button>

    <button type="button" class="ai-hub-card" on:click={() => { showFolderManager = !showFolderManager; }}>
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x1F4C2;</span>
        <span class="ai-hub-link">{showFolderManager ? $t('settings.actions.close') : $t('settings.actions.manage')}</span>
      </div>
      <strong>{$t('settings.cards.folders.title')}</strong>
      <p>{$t('settings.cards.folders.desc')}</p>
    </button>

    <button type="button" class="ai-hub-card" on:click={onToggleHistory}>
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x1F4DC;</span>
        <span class="ai-hub-link">{historyTrackingEnabled ? $t('settings.actions.enabled') : $t('settings.actions.disabled')}</span>
      </div>
      <strong>{$t('settings.cards.history.title')}</strong>
      <p>{historyTrackingEnabled ? $t('settings.cards.history.enabledDesc') : $t('settings.cards.history.disabledDesc')}</p>
    </button>

    <button type="button" class="ai-hub-card" on:click={onInstallAgg} disabled={aggAvailable}>
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x1F3AC;</span>
        <span class="ai-hub-link">{aggAvailable ? $t('settings.actions.installed') : $t('settings.actions.install')}</span>
      </div>
      <strong>{$t('settings.cards.agg.title')}</strong>
      <p>{#if aggAvailable}{$t('settings.cards.agg.installedDesc')}{:else if aggStatus === 'incompatible'}{$t('settings.cards.agg.incompatibleDesc')}{:else}{$t('settings.cards.agg.missingDesc')}{/if}</p>
    </button>

    <button type="button" class="ai-hub-card" on:click={onCheckUpdates} disabled={updateChecking}>
      <div class="ai-hub-card-top">
        <span class="ai-hub-icon">&#x21E7;</span>
        <span class="ai-hub-link">{updateChecking ? $t('settings.actions.checking') : (updateInfo?.updateAvailable ? $t('settings.actions.available') : $t('settings.actions.check'))}</span>
      </div>
      <strong>{$t('settings.cards.updates.title')}</strong>
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
    </button>
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
