<script>
  import { onMount, onDestroy } from 'svelte';
  import { t as tr } from './lib/i18n.js';
  import { GetLoadedSessionData, GetSSHTerminalLabel, ListRecordings, IsAggInstalled, IsHistoryTrackingEnabled, SetHistoryTracking, WriteToTerminal } from '../wailsjs/go/main/App';
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
  import { terminals, activeTerminalId, layoutTree, terminalMap, visibleTerminalCount } from './lib/stores/terminalStore.js';
  import { currentPage, errorMessage, showAIMenu, notesPanelOpen, notesPanelWidth, showFolderManager, historyTrackingEnabled, historyConsentDismissed, transcriptViewerState } from './lib/stores/uiStore.js';
  import { templates, templateToEdit, templatePromptState, showTemplatePicker, showWorkflowPicker, workflowPickerPlaybooks, workflowPickerLoading } from './lib/stores/templateStore.js';
  import { updateInfo, updateChecking, updateDownloading, updateProgress, updateInstalled } from './lib/stores/updateStore.js';
  import { recordingList, aggAvailable, aggStatus, downloadingAgg, aggDownloadInfo, terminalSessionFoldersOpen, customFolders, newFolderName } from './lib/stores/sessionStore.js';
  import { sshProfiles, showSSHProfileModal, sshSecretBackend, sshConnecting, hostKeyVerifyState, fileBrowserRemoteTerminalId, fileBrowserRemoteLabel } from './lib/stores/sshStore.js';
  import { aiPanelState, showFunctionCatalog, functionCatalog, showAISettings, aiProviders, aiSettings, aiToolFlowConfig, aiToolFlowLists } from './lib/stores/aiStore.js';
  import { groupedSidebarTerminals } from './lib/terminals/sidebarGroups';
  import { dedupeSavedSessionTerminals } from './lib/util';
  import { generateTmuxSessionName } from './lib/terminals/tmuxLifecycle';
  import { checkForUpdates, downloadUpdate, openUpdatePage, restartApp } from './lib/actions/updateActions.js';
  import { assignTerminalToFolder as assignTerminalToFolderAction, createFolder, deleteFolder as deleteFolderAction, loadCustomFolders, renameFolder, toggleTerminalFolder } from './lib/actions/folderActions.js';
  import { runAggDownload } from './lib/actions/aggActions.js';
  import { applyAISettingsDefaults, closeAISettings, getAIToolPromptPreview, getEditablePromptIntroPreview, isUsingDefaultPromptIntroPreview, loadAISettingsConfig, openAISettings, saveAISettings, setDevOpsPrePromptExample, toggleAIMenu } from './lib/actions/aiSettingsActions.js';
  import { createDragDropHandlers } from './lib/actions/dragDrop.js';
  import { createKeydownHandler } from './lib/actions/keyboardShortcuts.js';
  import { closeTerminalSearch, dismissRestoreSummary, terminalSearchNext, terminalSearchPrev, toggleTerminalSearch, updateTerminalSearchQuery } from './lib/actions/terminalSearchActions.js';
  import { applyTemplate, closeTemplatePrompt, handleTemplatePromptFieldChange, loadTemplatesFromBackend, runWorkflowFromPicker, submitTemplatePrompt, toggleWorkflowPicker } from './lib/actions/templateActions.js';
  import { createSSHActions, loadSSHProfiles, openSSHProfilePicker } from './lib/actions/sshActions.js';
  import { addTerminal, splitTerminal, removeTerminal, toggleRecording, reconnectSSH, toggleMinimize, startEditingName, saveTerminalName, handleResize, reinitializeTerminals, createTerminalInstance, cleanupTerminalResources } from './lib/actions/terminalActions.js';
  import { persistTerminalState, scheduleSessionSave, loadTranscriptExcerpt, clearSessionSaveTimer } from './lib/actions/sessionActions.js';

  const isWindowsPlatform = typeof navigator !== 'undefined' && navigator.userAgent.includes('Windows');
  const tmuxCapableTerminalTypes = new Set(['bash', 'zsh', 'wsl']);
  const defaultTerminalTypeStorageKey = 'mimir-default-terminal-type';

  function readDefaultTerminalType() {
    try {
      const saved = localStorage.getItem(defaultTerminalTypeStorageKey);
      if (saved) return saved;
    } catch (error) {
      console.error('Failed to read default terminal type:', error);
    }
    return preferredTerminalType()?.value || (isWindowsPlatform ? 'wsl' : 'bash');
  }

  function preferredTerminalType(options = availableTerminalTypes) {
    const order = isWindowsPlatform ? ['wsl', 'bash', 'powershell', 'cmd'] : ['bash', 'zsh'];
    return order.map((value) => options.find((option) => option.value === value)).find(Boolean) || options[0];
  }

  function persistDefaultTerminalType() {
    try {
      localStorage.setItem(defaultTerminalTypeStorageKey, selectedTerminalType);
    } catch (error) {
      console.error('Failed to save default terminal type:', error);
    }
  }

  function isAuditPage(page = $currentPage) {
    return ['historyDashboard', 'activityLogs', 'recordings'].includes(page);
  }

  let availableTerminalTypes = isWindowsPlatform
    ? [
        { value: 'cmd', label: 'CMD' },
        { value: 'powershell', label: 'PowerShell' },
        { value: 'wsl', label: 'WSL' },
        { value: 'bash', label: 'Bash' },
        { value: 'zsh', label: 'Zsh' },
        { value: 'ssh', label: 'SSH' }
      ]
    : [
        { value: 'bash', label: 'Bash' },
        { value: 'zsh', label: 'Zsh' },
        { value: 'ssh', label: 'SSH' }
      ];

  let selectedTerminalType = readDefaultTerminalType();
  let workflowBuilderQueuedEntry = null;
  let workflowBuilderQueuedEntries = [];

  async function loadAvailableTerminalTypes() {
    try {
      const detected = await window['go']['main']['App']['GetAvailableTerminalTypes']();
      if (Array.isArray(detected) && detected.length > 0) {
        availableTerminalTypes = detected;
        if (!availableTerminalTypes.some((option) => option.value === selectedTerminalType)) {
          const preferred = preferredTerminalType(availableTerminalTypes);
          selectedTerminalType = preferred.value;
          persistDefaultTerminalType();
        }
      }
    } catch (error) {
      console.error('Failed to load terminal types:', error);
    }
  }

  $: sidebarGroups = groupedSidebarTerminals($terminals, $customFolders);

  function assignTerminalToFolder(terminalId, folderId) {
    assignTerminalToFolderAction(terminalId, folderId, persistTerminalState);
  }

  function deleteFolder(folderId) {
    return deleteFolderAction(folderId, persistTerminalState);
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

  function doAddTerminal(terminalTypeParam, nameParam, minimized = false, initialPath = '') {
    return addTerminal(terminalTypeParam, nameParam, minimized, initialPath, {
      selectedTerminalType,
      openSSHProfilePicker,
    });
  }

  function openTranscriptViewer(id) {
    const term = $terminals.find((t) => t.id === id);
    $transcriptViewerState = {
      resumeId: term?.resumeId || '',
      label: term?.name || '',
    };
  }

  function openTranscriptBrowser() {
    $transcriptViewerState = { resumeId: '', label: '' };
  }

  function closeTranscriptViewer() {
    $transcriptViewerState = null;
  }

  function openAIPanel(mode) {
    if ($activeTerminalId === null) {
      $errorMessage = 'Please select a terminal first.';
      return;
    }
    const activeTerminal = $terminals.find(t => t.id === $activeTerminalId);
    if (!activeTerminal) {
      $errorMessage = 'Active terminal not found.';
      return;
    }
    $aiPanelState = {
      mode,
      terminalId: activeTerminal.id,
      terminalType: activeTerminal.type,
      terminalName: activeTerminal.name,
      goal: '',
      result: '',
      loading: false,
    };
    $showAIMenu = false;
  }

  function closeAIPanel() {
    $aiPanelState = null;
  }

  async function openFunctionCatalog() {
    try {
      const payload = await window['go']['main']['App']['GetFunctionCatalogJSON']();
      $functionCatalog = JSON.parse(payload);
      $showFunctionCatalog = true;
      $showAIMenu = false;
      $errorMessage = '';
    } catch (error) {
      $errorMessage = `Failed to load function catalog: ${error.message || error}`;
    }
  }

  function getDiscoveryTerminalType() {
    const activeTerminal = $terminals.find((terminal) => terminal.id === $activeTerminalId);
    return activeTerminal?.type || selectedTerminalType || (isWindowsPlatform ? 'powershell' : 'bash');
  }

  function queueWorkflowFromCatalog(entries) {
    if (entries.length === 1) {
      workflowBuilderQueuedEntry = entries[0];
      workflowBuilderQueuedEntries = [];
    } else {
      workflowBuilderQueuedEntries = entries;
      workflowBuilderQueuedEntry = null;
    }
    $showFunctionCatalog = false;
    openPage("workflowBuilder");
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
      if (remote && terminalId) {
        await window['go']['main']['App']['ImportNoteFromRemote'](terminalId, path);
      } else {
        await window['go']['main']['App']['ImportNoteFromLocal'](path);
      }
      $notesPanelOpen = true;
      if ($currentPage !== 'terminals') {
        await openPage('terminals');
      }
      setTimeout(handleResize, 50);
    } catch (err) {
      $errorMessage = `Failed to import note: ${err}`;
    }
  }

  function startNotesDrag(e) {
    e.preventDefault();
    const startX = e.clientX;
    const startWidth = $notesPanelWidth;
    const onMove = (ev) => {
      $notesPanelWidth = Math.max(250, Math.min(800, startWidth + (startX - ev.clientX)));
    };
    const onUp = () => {
      localStorage.setItem('mimir-notes-width', String($notesPanelWidth));
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
      handleResize();
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }

  const {
    handleDragStart,
    handleDragOver,
    handleDragLeave,
    handleDrop,
    handleDragEnd,
  } = createDragDropHandlers({ reinitializeTerminals });

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
  });

  async function openPage(page) {
    $currentPage = page;

    if (page === "terminals") {
      await reinitializeTerminals();
      return;
    }

    if (page === "recordings" || page === "settings") {
      $aggAvailable = await IsAggInstalled().catch(() => false);
      $aggStatus = await window['go']['main']['App']['GetAggStatus']().catch(() => 'missing');
    }

    if (page === "recordings") {
      try {
        $recordingList = await ListRecordings();
      } catch (e) {
        console.error('Failed to load recordings:', e);
      }
    }

    if (page === "templateManager") {
      try {
        await loadTemplatesFromBackend();
      } catch (error) {
        $errorMessage = `Failed to load templates: ${error.message || error}`;
      }
    }

    if (page === "fileBrowser") {
      const activeTerm = $terminals.find(t => t.id === $activeTerminalId);
      if (activeTerm && activeTerm.type === 'ssh' && !activeTerm.disconnected) {
        try {
          const label = await GetSSHTerminalLabel(activeTerm.id);
          if (label) {
            $fileBrowserRemoteTerminalId = activeTerm.id;
            $fileBrowserRemoteLabel = label;
            return;
          }
        } catch (e) {
          // Fall through to local mode
        }
      }
      $fileBrowserRemoteTerminalId = 0;
      $fileBrowserRemoteLabel = '';
    }
  }

  async function restoreLocalTerminal(saved) {
    const tmuxSessionName = tmuxCapableTerminalTypes.has(saved.type)
      ? (saved.tmuxSessionName || generateTmuxSessionName('mimir'))
      : '';
    const startWithOptions = window['go']?.['main']?.['App']?.['StartTerminalWithOptions'];
    const id = typeof startWithOptions === 'function'
      ? await startWithOptions(saved.type, tmuxSessionName)
      : await window['go']['main']['App']['StartTerminal'](saved.type);
    if (!id) return;
    const newLeaf = { type: 'leaf', terminalId: id };
    if (saved.minimized) {
      // minimized: don't add to layout tree
    } else if ($layoutTree === null) {
      $layoutTree = newLeaf;
    } else {
      $layoutTree = { type: 'split', direction: 'horizontal', ratio: 0.5, children: [$layoutTree, newLeaf] };
    }
    const restoreClass = ['bash', 'zsh', 'wsl'].includes(saved.type) ? 'rehydrated' : 'transcript-restored';
    const newTerminal = await createTerminalInstance(
      id, saved.type, saved.name, saved.minimized, '', true, tmuxSessionName, saved.resumeId || '', restoreClass,
    );
    const restoredTranscript = await loadTranscriptExcerpt(newTerminal.resumeId);
    const folderId = saved.folderId || '';
    $terminals = $terminals.map((t) => t.id === newTerminal.id ? { ...t, restoredTranscript, restoreClass, restoreDismissed: false, folderId } : t);
    persistTerminalState({ ...newTerminal, restoreClass, folderId });
    await reinitializeTerminals();
  }

  async function restoreSSHTerminal(profile, saved) {
    try {
      const startSSH = window['go']['main']['App']['StartSSHTerminal'];
      const id = await startSSH(profile.id);
      if (!id) return;
      const newLeaf = { type: 'leaf', terminalId: id };
      if (saved.minimized) {
        // minimized: don't add to layout tree
      } else if ($layoutTree === null) {
        $layoutTree = newLeaf;
      } else {
        $layoutTree = { type: 'split', direction: 'horizontal', ratio: 0.5, children: [$layoutTree, newLeaf] };
      }
      const name = saved.name || `SSH: ${profile.name}`;
      const restoreClass = 'live-restored';
      const newTerminal = await createTerminalInstance(
        id, 'ssh', name, saved.minimized, profile.id, true, saved.tmuxSessionName || '', saved.resumeId || '', restoreClass,
      );
      const restoredTranscript = await loadTranscriptExcerpt(newTerminal.resumeId);
      const folderId = saved.folderId || '';
      $terminals = $terminals.map((t) => t.id === newTerminal.id ? { ...t, restoredTranscript, restoreClass, restoreDismissed: false, folderId } : t);
      persistTerminalState({ ...newTerminal, type: 'ssh', sshProfileId: profile.id, restoreClass, folderId });
      await reinitializeTerminals();
    } catch (e) {
      console.warn(`Failed to restore SSH terminal for ${profile.name}: ${e.message || e}`);
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
    window.addEventListener('resize', handleResize);
    window.addEventListener('error', handleGlobalError);
    window.addEventListener('unhandledrejection', handleGlobalError);
    await loadSSHProfiles();
    try {
      await loadAvailableTerminalTypes();
      await loadTemplatesFromBackend();
      await loadCustomFolders();
      $historyTrackingEnabled = await IsHistoryTrackingEnabled();
      await loadAISettingsConfig();

      try {
        const pendingRaw = await window['go']['main']['App']['GetPendingUpdate']();
        const pending = pendingRaw ? JSON.parse(pendingRaw) : null;
        if (pending?.version) $updateInstalled = true;
      } catch (_) {}

      const savedSession = await GetLoadedSessionData();
      const savedTerminals = dedupeSavedSessionTerminals(savedSession?.terminals || []);
      if (savedTerminals.length > 0) {
        for (const saved of savedTerminals) {
          if (saved.type === 'ssh' && saved.sshProfileId) {
            const profile = $sshProfiles.find(p => p.id === saved.sshProfileId);
            if (profile) await restoreSSHTerminal(profile, saved);
          } else if (['bash', 'zsh', 'wsl', 'cmd', 'powershell'].includes(saved.type)) {
            await restoreLocalTerminal(saved);
          }
        }
      }
      if ($terminals.length === 0) doAddTerminal();
      else scheduleSessionSave(50);
    } catch (error) {
      $errorMessage = `Failed to load templates or session: ${error.message || error}`;
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
    window.removeEventListener('resize', handleResize);
    window.removeEventListener('error', handleGlobalError);
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
    bind:currentPage={$currentPage}
    {availableTerminalTypes}
    bind:selectedTerminalType
    bind:aiSettings={$aiSettings}
    visibleTerminalCount={$visibleTerminalCount}
    errorMessage={$errorMessage}
    bind:layoutTree={$layoutTree}
    terminalMap={$terminalMap}
    bind:activeTerminalId={$activeTerminalId}
    bind:notesPanelOpen={$notesPanelOpen}
    notesPanelWidth={$notesPanelWidth}
    bind:terminals={$terminals}
    showAIMenu={$showAIMenu}
    bind:templateToEdit={$templateToEdit}
    bind:templates={$templates}
    bind:fileBrowserRemoteTerminalId={$fileBrowserRemoteTerminalId}
    bind:fileBrowserRemoteLabel={$fileBrowserRemoteLabel}
    {workflowBuilderQueuedEntry}
    {workflowBuilderQueuedEntries}
    bind:aiToolFlowConfig={$aiToolFlowConfig}
    bind:recordingList={$recordingList}
    aggAvailable={$aggAvailable}
    aggStatus={$aggStatus}
    bind:showFolderManager={$showFolderManager}
    bind:historyTrackingEnabled={$historyTrackingEnabled}
    updateChecking={$updateChecking}
    updateInfo={$updateInfo}
    updateDownloading={$updateDownloading}
    updateProgress={$updateProgress}
    updateInstalled={$updateInstalled}
    customFolders={$customFolders}
    bind:newFolderName={$newFolderName}
    {persistDefaultTerminalType}
    addTerminal={doAddTerminal}
    {toggleAIMenu}
    {openAIPanel}
    {splitTerminal}
    {toggleMinimize}
    {removeTerminal}
    {reconnectSSH}
    {startEditingName}
    {saveTerminalName}
    {handleResize}
    {handleDragStart}
    {handleDragOver}
    {handleDragLeave}
    {handleDrop}
    {handleDragEnd}
    {updateTerminalSearchQuery}
    {terminalSearchNext}
    {terminalSearchPrev}
    {closeTerminalSearch}
    {dismissRestoreSummary}
    {toggleRecording}
    {openTranscriptViewer}
    {startNotesDrag}
    {openPage}
    {insertFileIntoActiveTerminal}
    {handleOpenInNotes}
    {getEditablePromptIntroPreview}
    {isUsingDefaultPromptIntroPreview}
    {openFunctionCatalog}
    {openAISettings}
    {checkForUpdates}
    {openUpdatePage}
    {downloadUpdate}
    {restartApp}
    {createFolder}
    {renameFolder}
    {deleteFolder}
    onError={(msg) => { $errorMessage = msg; }}
    onAggDownloadInfo={(info) => { $aggDownloadInfo = info; }}
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

  <AppModals
    bind:showTemplatePicker={$showTemplatePicker}
    templates={$templates}
    {applyTemplate}
    bind:showWorkflowPicker={$showWorkflowPicker}
    workflowPickerPlaybooks={$workflowPickerPlaybooks}
    workflowPickerLoading={$workflowPickerLoading}
    {runWorkflowFromPicker}
    closeWorkflowPicker={() => { $showWorkflowPicker = false; }}
    bind:templatePromptState={$templatePromptState}
    {closeTemplatePrompt}
    {submitTemplatePrompt}
    {handleTemplatePromptFieldChange}
    bind:aiPanelState={$aiPanelState}
    terminals={$terminals}
    {closeAIPanel}
    onError={(msg) => { $errorMessage = msg; }}
    bind:showAISettings={$showAISettings}
    bind:aiSettings={$aiSettings}
    bind:aiToolFlowConfig={$aiToolFlowConfig}
    bind:aiToolFlowLists={$aiToolFlowLists}
    aiProviders={$aiProviders}
    promptPreview={getAIToolPromptPreview()}
    {closeAISettings}
    {saveAISettings}
    {applyAISettingsDefaults}
    {setDevOpsPrePromptExample}
    bind:showFunctionCatalog={$showFunctionCatalog}
    functionCatalog={$functionCatalog}
    discoveryTerminalType={getDiscoveryTerminalType()}
    {queueWorkflowFromCatalog}
    closeFunctionCatalog={() => { $showFunctionCatalog = false; }}
    bind:showSSHProfileModal={$showSSHProfileModal}
    sshProfiles={$sshProfiles}
    sshSecretBackend={$sshSecretBackend}
    sshConnecting={$sshConnecting}
    {connectSSHProfile}
    closeSSHProfileModal={() => { $showSSHProfileModal = false; }}
    onProfilesChanged={async (p) => { $sshProfiles = Array.isArray(p) ? p : []; await loadSSHProfiles(); }}
    hostKeyVerifyState={$hostKeyVerifyState}
    {acceptHostKey}
    {rejectHostKey}
    bind:aggDownloadInfo={$aggDownloadInfo}
    downloadingAgg={$downloadingAgg}
    cancelAggDownload={() => { $aggDownloadInfo = null; }}
    {runAggDownload}
    bind:transcriptViewerState={$transcriptViewerState}
    {closeTranscriptViewer}
  />
</main>
