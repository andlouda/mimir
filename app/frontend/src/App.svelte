<script>
  import { onMount, onDestroy } from 'svelte';
  import { startPaneResourceSampling, stopPaneResourceSampling } from './lib/actions/paneResources.js';
  import { t as tr } from './lib/i18n.js';
  import { GetFunctionCatalogJSON, GetLoadedSessionData, GetPendingUpdate, GetTmuxIntegrationMode, ImportNoteFromLocal, ImportNoteFromRemote, IsHistoryTrackingEnabled, optional, SetHistoryTracking, StartSSHTerminal, StartTerminal, WriteToTerminal } from './lib/backend.js';
  import { EventsOn } from '../wailsjs/runtime';
  import SecretUnlockGate from './lib/SecretUnlockGate.svelte';
  import Sidebar from './lib/Sidebar.svelte';
  import AppMainContent from './lib/AppMainContent.svelte';
  import AppModals from './lib/AppModals.svelte';
  import './styles/base.css';
  import './styles/sidebar.css';
  import './styles/main.css';
  import './styles/ai-hub.css';
  import './styles/history-dashboard.css';
  import './styles/overlays.css';
  import './styles/recording-player.css';
  import './styles/template-manager.css';
  import './styles/workflow-builder.css';
  import { terminals, activeTerminalId, layoutTree } from './lib/stores/terminalStore.js';
  import { loadPromptMode, currentPage, errorMessage, showAIMenu, notesPanelOpen, historyTrackingEnabled, historyConsentDismissed, transcriptViewerState, tmuxIntegrationMode, notesOpenRequest } from './lib/stores/uiStore.js';
  import { templates, templateToEdit } from './lib/stores/templateStore.js';
  import { updateDownloading, updateProgress, updateInstalled } from './lib/stores/updateStore.js';
  import { recordingList, aggAvailable, terminalSessionFoldersOpen, customFolders } from './lib/stores/sessionStore.js';
  import { sshProfiles, fileBrowserRemoteTerminalId, fileBrowserRemoteLabel } from './lib/stores/sshStore.js';
  import { showFunctionCatalog, functionCatalog, aiToolFlowConfig } from './lib/stores/aiStore.js';
  import { loadAgentDetectionSetting, loadAgentAnnotations } from './lib/stores/agentStore.js';
  import { loadAgentWorkspace } from './lib/stores/agentWorkspaceStore.js';
  import { groupedSidebarTerminals } from './lib/terminals/sidebarGroups';
  import { dedupeSavedSessionTerminals } from './lib/util';
  import { droppedPathsText, isFileDrop } from './lib/terminals/droppedPaths.js';
  import { appendLeaf, rebuildLayout, collectLeafIds } from './lib/terminals/layoutTree.js';
  import { generateTmuxSessionName } from './lib/terminals/tmuxLifecycle';
  import { assignTerminalToFolder as assignTerminalToFolderAction, loadCustomFolders, toggleTerminalFolder } from './lib/actions/folderActions.js';
  import { openPage } from './lib/actions/navigationActions.js';
  import { loadAvailableTerminalTypes } from './lib/actions/terminalTypeActions.js';
  import { getEditablePromptIntroPreview, isUsingDefaultPromptIntroPreview, loadAISettingsConfig, openAISettings } from './lib/actions/aiSettingsActions.js';
  import { createKeydownHandler } from './lib/actions/keyboardShortcuts.js';
  import { closeTerminalSearch, toggleTerminalSearch } from './lib/actions/terminalSearchActions.js';
  import { loadTemplatesFromBackend, toggleWorkflowPicker } from './lib/actions/templateActions.js';
  import { createSSHActions, loadSSHProfiles, openSSHProfilePicker } from './lib/actions/sshActions.js';
  import { addTerminal, toggleMinimize, handleResize, reinitializeTerminals, createTerminalInstance, cleanupTerminalResources } from './lib/actions/terminalActions.js';
  import { persistTerminalState, scheduleSessionSave, loadTranscriptExcerpt, clearSessionSaveTimer, layoutKeyFor, enableLayoutPersistence } from './lib/actions/sessionActions.js';
  import { applyTerminalBackground } from './lib/actions/backgroundActions.js';
  import {} from './lib/actions/agentActions.js';
  import { parseBackground } from './lib/terminals/background.js';

  const tmuxCapableTerminalTypes = new Set(['bash', 'zsh', 'wsl']);

  function isAuditPage(page = $currentPage) {
    return ['historyDashboard', 'activityLogs', 'recordings'].includes(page);
  }


  $: sidebarGroups = groupedSidebarTerminals($terminals, $customFolders);

  function assignTerminalToFolder(terminalId, folderId) {
    assignTerminalToFolderAction(terminalId, folderId, persistTerminalState);
  }

  function selectSidebarTerminal(term) {
    if (!term) return;
    if (term.minimized) {
      toggleMinimize(term.id);
    }
    $activeTerminalId = term.id;
    openPage('terminals');
    setTimeout(handleResize, 50);
  }

  function openTerminals() {
    $templateToEdit = null;
    openPage('terminals');
  }



  function openTranscriptBrowser() {
    $transcriptViewerState = { resumeId: '', label: '' };
  }

  async function openFunctionCatalog() {
    try {
      const payload = await GetFunctionCatalogJSON();
      $functionCatalog = JSON.parse(payload);
      $showFunctionCatalog = true;
      $showAIMenu = false;
      $errorMessage = '';
    } catch (error) {
      $errorMessage = `Failed to load function catalog: ${error.message || error}`;
    }
  }

  async function insertFileIntoActiveTerminal(event) {
    if ($activeTerminalId === null) {
      $errorMessage = 'Please select a terminal first.';
      return;
    }
    try {
      await WriteToTerminal($activeTerminalId, event.detail.content);
      $errorMessage = '';
    } catch (error) {
      $errorMessage = `Failed to insert file into terminal: ${error.message || error}`;
    }
  }

  async function handleOpenInNotes(event) {
    const { path, remote, terminalId } = event.detail;
    try {
      let imported = '';
      if (remote && terminalId) {
        imported = await ImportNoteFromRemote(terminalId, path);
      } else {
        imported = await ImportNoteFromLocal(path);
      }
      if (imported) $notesOpenRequest = imported;
      $notesPanelOpen = true;
      if ($currentPage !== 'terminals') {
        await openPage('terminals');
      }
      setTimeout(handleResize, 50);
    } catch (err) {
      $errorMessage = `Failed to import note: ${err}`;
    }
  }

  const {
    connectSSHProfile,
    acceptHostKey,
    rejectHostKey,
  } = createSSHActions({
    createTerminalInstance,
    persistTerminalState,
    reinitializeTerminals,
  });

  const handleGlobalKeydown = createKeydownHandler({
    handleResize,
    toggleTerminalSearch,
    toggleWorkflowPicker,
    closeTerminalSearch,
    addTerminal: () => addTerminal(),
    toggleMinimize,
  });

  // Retries a restore step when the backend's start cap answers with a
  // rate-limit error (a large saved session starts many terminals at once).
  async function restoreWithRetry(step, attempts = 4) {
    for (let i = 0; ; i++) {
      try {
        return await step();
      } catch (error) {
        const rateLimited = /rate limit/i.test(String(error?.message || error));
        if (!rateLimited || i >= attempts - 1) throw error;
        await new Promise((resolve) => setTimeout(resolve, 2500 * (i + 1)));
      }
    }
  }

  async function restoreLocalTerminal(saved) {
    const tmuxSessionName = tmuxCapableTerminalTypes.has(saved.type)
      ? (saved.tmuxSessionName || generateTmuxSessionName('mimir'))
      : '';
    const startWithOptions = optional('StartTerminalWithOptions');
    const id = typeof startWithOptions === 'function'
      ? await startWithOptions(saved.type, tmuxSessionName)
      : await StartTerminal(saved.type);
    if (!id) return null;
    // The layout is rebuilt once after every terminal is back (see onMount).
    const restoreClass = ['bash', 'zsh', 'wsl'].includes(saved.type) ? 'rehydrated' : 'transcript-restored';
    const newTerminal = await createTerminalInstance(
      id, saved.type, saved.name, saved.minimized, '', true, tmuxSessionName, saved.resumeId || '', restoreClass,
    );
    if (!newTerminal) throw new Error($tr('appTerminals.restoreExited'));
    const restoredTranscript = await loadTranscriptExcerpt(newTerminal.resumeId);
    const folderId = saved.folderId || '';
    $terminals = $terminals.map((t) => t.id === newTerminal.id ? { ...t, restoredTranscript, restoreClass, restoreDismissed: false, folderId } : t);
    persistTerminalState({ ...newTerminal, restoreClass, folderId });
    if (saved.background) applyTerminalBackground(newTerminal.id, parseBackground(saved.background));
    await reinitializeTerminals();
    return { id, minimized: !!saved.minimized, key: layoutKeyFor({ ...newTerminal, resumeId: newTerminal.resumeId || saved.resumeId || '', tmuxSessionName }) };
  }

  async function restoreSSHTerminal(profile, saved) {
    {
      const startSSH = StartSSHTerminal;
      const id = await startSSH(profile.id);
      if (!id) return null;
      const name = saved.name || `SSH: ${profile.name}`;
      const restoreClass = 'live-restored';
      const newTerminal = await createTerminalInstance(
        id, 'ssh', name, saved.minimized, profile.id, true, saved.tmuxSessionName || '', saved.resumeId || '', restoreClass,
      );
      if (!newTerminal) throw new Error($tr('appTerminals.restoreExited'));
      const restoredTranscript = await loadTranscriptExcerpt(newTerminal.resumeId);
      const folderId = saved.folderId || '';
      $terminals = $terminals.map((t) => t.id === newTerminal.id ? { ...t, restoredTranscript, restoreClass, restoreDismissed: false, folderId } : t);
      persistTerminalState({ ...newTerminal, type: 'ssh', sshProfileId: profile.id, restoreClass, folderId });
      if (saved.background) applyTerminalBackground(newTerminal.id, parseBackground(saved.background));
      await reinitializeTerminals();
      return { id, minimized: !!saved.minimized, key: layoutKeyFor({ ...newTerminal, resumeId: newTerminal.resumeId || saved.resumeId || '', tmuxSessionName: saved.tmuxSessionName || '' }) };
    }
  }

  // A file dropped onto the window would navigate the webview away and
  // unmount every terminal. Cancel that everywhere; when the drop carries
  // file URIs, paste the quoted paths into the active terminal instead.
  function handleWindowDragOver(event) {
    if (isFileDrop(event.dataTransfer)) {
      event.preventDefault();
      try { event.dataTransfer.dropEffect = 'copy'; } catch { /* ignore */ }
    }
  }
  function handleWindowDrop(event) {
    if (!isFileDrop(event.dataTransfer)) return;
    event.preventDefault();
    const id = $activeTerminalId;
    const target = id != null ? $terminals.find((t) => t.id === id) : null;
    const text = droppedPathsText(event.dataTransfer.getData('text/uri-list'), target?.type || '');
    if (text && id != null && $currentPage === 'terminals') {
      WriteToTerminal(id, text).catch((error) => console.error('Could not paste dropped paths:', error));
    }
  }
  // WebView2 / WebKitGTK zoom the page on Ctrl+wheel outside the panes;
  // Mimir's own terminal zoom (SplitPane) is the only zoom.
  function handleWindowWheel(event) {
    if (event.ctrlKey) event.preventDefault();
  }

  function handleNativeContextMenu(event) {
    const t = event.target;
    const editable = t && (t.closest?.('input, textarea, [contenteditable="true"]'));
    if (!editable) event.preventDefault();
  }
  function handleBeforeUnload(event) {
    if ($terminals.length > 0) {
      event.preventDefault();
      event.returnValue = '';
    }
  }

  function handleGlobalError(event) {
    const msg = event?.reason?.message || event?.message || 'Unknown error';
    console.error('[mimir] unhandled error:', msg);
    if (!$errorMessage) {
      $errorMessage = msg;
    }
    if (event?.preventDefault) event.preventDefault();
  }

  onMount(async () => {
    startPaneResourceSampling();
    window.addEventListener('resize', handleResize);
    window.addEventListener('error', handleGlobalError);
    window.addEventListener('unhandledrejection', handleGlobalError);
    // The webview's own context menu offers "Reload", which drops every
    // terminal; keep it to text fields, Mimir draws its own menu in panes.
    window.addEventListener('contextmenu', handleNativeContextMenu);
    window.addEventListener('beforeunload', handleBeforeUnload);
    window.addEventListener('dragover', handleWindowDragOver);
    window.addEventListener('drop', handleWindowDrop);
    window.addEventListener('wheel', handleWindowWheel, { passive: false });
    // Every start-up loader runs on its own: a broken AI config or a
    // missing template must not skip the session restore below (which
    // used to leave the saved panes unrestored and then overwritten).
    const loaderErrors = [];
    const loaders = [
      ['SSH profiles', loadSSHProfiles],
      ['terminal types', loadAvailableTerminalTypes],
      ['templates', loadTemplatesFromBackend],
      ['folders', loadCustomFolders],
      ['history setting', async () => { $historyTrackingEnabled = await IsHistoryTrackingEnabled(); }],
      ['agent detection', loadAgentDetectionSetting],
      ['agent annotations', loadAgentAnnotations],
      ['prompt mode', loadPromptMode],
      ['tmux mode', async () => { $tmuxIntegrationMode = await GetTmuxIntegrationMode(); }, { quiet: true }],
      ['AI settings', loadAISettingsConfig],
      ['pending update', async () => {
        const pendingRaw = await GetPendingUpdate();
        const pending = pendingRaw ? JSON.parse(pendingRaw) : null;
        if (pending?.version) $updateInstalled = true;
      }],
    ];
    for (const [label, load, opts] of loaders) {
      try {
        await load();
      } catch (error) {
        console.warn(`Start-up loader failed (${label}):`, error);
        if (!opts?.quiet) loaderErrors.push(`${label}: ${error?.message || error}`);
      }
    }
    loadAgentWorkspace().catch(() => {}); // Errors remain visible in the workspace.
    if (loaderErrors.length) $errorMessage = `${$tr('appTerminals.loadersFailed')}\n${loaderErrors.join('\n')}`;
    try {
      const savedSession = await GetLoadedSessionData();
      const savedTerminals = dedupeSavedSessionTerminals(savedSession?.terminals || []);
      const notRestored = [];
      const unrestored = []; // saved entries that failed: kept by the backend for the next start
      const restored = []; // { id, minimized, key } in save order
      if (savedTerminals.length > 0) {
        for (const saved of savedTerminals) {
          // One failing terminal must not abort the rest of the restore;
          // the backend's start cap answers with a rate-limit error, which
          // is retried after a pause instead of dropping the terminal.
          try {
            await restoreWithRetry(async () => {
              let entry = null;
              if (saved.type === 'ssh' && saved.sshProfileId) {
                const profile = $sshProfiles.find(p => p.id === saved.sshProfileId);
                if (!profile) throw new Error($tr('appTerminals.restoreProfileMissing'));
                entry = await restoreSSHTerminal(profile, saved);
              } else if (['bash', 'zsh', 'wsl', 'cmd', 'powershell'].includes(saved.type)) {
                entry = await restoreLocalTerminal(saved);
              }
              // The saved key is what the layout was serialised with; the
              // new terminal keeps resume id and tmux name, so they match.
              if (entry) restored.push({ ...entry, savedKey: layoutKeyFor(saved) });
            });
          } catch (error) {
            notRestored.push(`${saved.name || saved.type}: ${error?.message || error}`);
            unrestored.push(saved);
          }
        }
      }
      // Tell the backend which entries did not come back: they stay in the
      // session file and are tried again next time instead of vanishing.
      try {
        const finish = optional('FinishSessionRestore');
        if (typeof finish === 'function') await finish(JSON.stringify(unrestored));
      } catch (error) {
        console.warn('Could not report the restore outcome:', error);
      }
      // Rebuild the arrangement from the saved layout (keys → new ids);
      // anything visible that the layout does not mention is appended with
      // an equal share. Without a saved layout the panes form a balanced row.
      let tree = null;
      try {
        const savedLayout = savedSession?.layout ? JSON.parse(savedSession.layout) : null;
        const byKey = new Map();
        for (const r of restored) { if (!r.minimized) { byKey.set(r.savedKey, r.id); byKey.set(r.key, r.id); } }
        tree = rebuildLayout(savedLayout, (key) => byKey.get(key) ?? null);
      } catch (error) {
        console.warn('Saved layout unreadable, using a balanced row:', error);
        tree = null;
      }
      const placed = new Set(tree ? collectLeafIds(tree) : []);
      for (const r of restored) {
        if (!r.minimized && !placed.has(r.id)) tree = appendLeaf(tree, { type: 'leaf', terminalId: r.id });
      }
      $layoutTree = tree;
      if (restored.length) await reinitializeTerminals();
      enableLayoutPersistence();
      if (notRestored.length) $errorMessage = `${$tr('appTerminals.restoreFailed')}\n${notRestored.join('\n')}\n${$tr('appTerminals.restoreRetryHint')}`;
      if ($terminals.length === 0) addTerminal();
      else scheduleSessionSave(50);
      enableLayoutPersistence();
    } catch (error) {
      $errorMessage = `${$tr('appTerminals.restoreCrashed')} ${error.message || error}`;
      if ($terminals.length === 0) addTerminal();
    }
  });

  const offUpdateProgress = EventsOn('update-progress', (data) => {
    try {
      const progress = JSON.parse(data);
      $updateProgress = progress;
      if (progress.stage === 'done') {
        $updateDownloading = false;
        $updateInstalled = true;
      } else if (progress.stage === 'error') {
        $updateDownloading = false;
        $errorMessage = `Update failed: ${progress.error}`;
      }
    } catch (_) {}
  });

  onDestroy(() => {
    stopPaneResourceSampling();
    window.removeEventListener('resize', handleResize);
    window.removeEventListener('error', handleGlobalError);
    window.removeEventListener('contextmenu', handleNativeContextMenu);
    window.removeEventListener('beforeunload', handleBeforeUnload);
    window.removeEventListener('dragover', handleWindowDragOver);
    window.removeEventListener('drop', handleWindowDrop);
    window.removeEventListener('wheel', handleWindowWheel);
    window.removeEventListener('unhandledrejection', handleGlobalError);
    offUpdateProgress();
    $terminals.forEach((term) => cleanupTerminalResources(term));
    clearSessionSaveTimer();
  });
</script>

<svelte:window on:keydown={handleGlobalKeydown} />

<SecretUnlockGate />

<main>
  <Sidebar
    currentPage={$currentPage}
    terminals={$terminals}
    sshProfiles={$sshProfiles}
    customFolders={$customFolders}
    groups={sidebarGroups}
    foldersOpen={$terminalSessionFoldersOpen}
    activeTerminalId={$activeTerminalId}
    openPage={openPage}
    openTerminals={openTerminals}
    openSSHProfilePicker={openSSHProfilePicker}
    refreshSSHProfiles={loadSSHProfiles}
    isAuditPage={isAuditPage}
    assignTerminalToFolder={assignTerminalToFolder}
    toggleTerminalFolder={toggleTerminalFolder}
    selectTerminal={selectSidebarTerminal}
    connectSSHProfile={connectSSHProfile}
    onResize={handleResize}
    openTranscripts={openTranscriptBrowser}
  />

  <AppMainContent
    selectTerminal={selectSidebarTerminal}
    bind:currentPage={$currentPage}
    bind:templateToEdit={$templateToEdit}
    bind:templates={$templates}
    bind:fileBrowserRemoteTerminalId={$fileBrowserRemoteTerminalId}
    bind:fileBrowserRemoteLabel={$fileBrowserRemoteLabel}
    bind:aiToolFlowConfig={$aiToolFlowConfig}
    bind:recordingList={$recordingList}
    aggAvailable={$aggAvailable}
    {openPage}
    {insertFileIntoActiveTerminal}
    {handleOpenInNotes}
    {getEditablePromptIntroPreview}
    {isUsingDefaultPromptIntroPreview}
    {openFunctionCatalog}
    {openAISettings}
    onError={(msg) => { $errorMessage = msg; }}
  />

  {#if !$historyTrackingEnabled && !$historyConsentDismissed}
    <div class="history-consent-banner">
      <p><strong>{$tr('appTerminals.historyConsentTitle')}</strong> — {$tr('appTerminals.historyConsentText')}</p>
      <div class="history-consent-actions">
        <button type="button" class="modal-primary-button" on:click={async () => { await SetHistoryTracking(true); $historyTrackingEnabled = true; $historyConsentDismissed = true; }}>{$tr('appTerminals.enable')}</button>
        <button type="button" class="modal-secondary-button" on:click={() => { $historyConsentDismissed = true; }}>{$tr('appTerminals.notNow')}</button>
      </div>
    </div>
  {/if}

  <AppModals />
</main>
