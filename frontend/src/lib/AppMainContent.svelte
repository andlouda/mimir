<script>
  import { SaveTemplate, UpdateTemplate, DeleteTemplate, ToggleFavorite, GetTemplates, WriteToTerminal, GetRecording, DeleteRecording, ExportRecordingScrubbed, ExportRecordingGIF, ExportRecordingTrimmed, ExportRecordingTrimmedGIF, ListRecordings, GetAggDownloadInfo, SetHistoryTracking } from '../../wailsjs/go/main/App';
  import TemplateManager from './TemplateManager.svelte';
  import FileBrowser from './FileBrowser.svelte';
  import WorkflowBuilder from './workflows/WorkflowBuilder.svelte';
  import ActivityLogViewer from './ActivityLogViewer.svelte';
  import HistoryDashboard from './HistoryDashboard.svelte';
  import RecordingPlayer from './RecordingPlayer.svelte';
  import TerminalsPage from './views/TerminalsPage.svelte';
  import AIHubView from './views/AIHubView.svelte';
  import SettingsView from './views/SettingsView.svelte';
  import { normalizeTemplates } from './templates/templateHelpers';
  import { shellQuotePath } from './util';

  export let currentPage = 'terminals';
  export let availableTerminalTypes = [];
  export let selectedTerminalType = '';
  export let aiSettings = {};
  export let visibleTerminalCount = 0;
  export let errorMessage = '';
  export let layoutTree = null;
  export let terminalMap = new Map();
  export let activeTerminalId = null;
  export let notesPanelOpen = false;
  export let notesPanelWidth = 380;
  export let terminals = [];
  export let showAIMenu = false;
  export let templateToEdit = null;
  export let templates = [];
  export let fileBrowserRemoteTerminalId = 0;
  export let fileBrowserRemoteLabel = '';
  export let workflowBuilderQueuedEntry = null;
  export let workflowBuilderQueuedEntries = [];
  export let aiToolFlowConfig = {};
  export let recordingList = [];
  export let aggAvailable = false;
  export let showFolderManager = false;
  export let historyTrackingEnabled = false;
  export let updateChecking = false;
  export let updateInfo = null;
  export let customFolders = [];
  export let newFolderName = '';

  export let persistDefaultTerminalType = () => {};
  export let addTerminal = () => {};
  export let toggleAIMenu = () => {};
  export let openAIPanel = () => {};
  export let splitTerminal = () => {};
  export let toggleMinimize = () => {};
  export let removeTerminal = () => {};
  export let reconnectSSH = () => {};
  export let startEditingName = () => {};
  export let saveTerminalName = () => {};
  export let handleResize = () => {};
  export let handleDragStart = () => {};
  export let handleDragOver = () => {};
  export let handleDragLeave = () => {};
  export let handleDrop = () => {};
  export let handleDragEnd = () => {};
  export let updateTerminalSearchQuery = () => {};
  export let terminalSearchNext = () => {};
  export let terminalSearchPrev = () => {};
  export let closeTerminalSearch = () => {};
  export let dismissRestoreSummary = () => {};
  export let toggleRecording = () => {};
  export let startNotesDrag = () => {};
  export let openPage = () => {};
  export let insertFileIntoActiveTerminal = () => {};
  export let handleOpenInNotes = () => {};
  export let getEditablePromptIntroPreview = () => '';
  export let isUsingDefaultPromptIntroPreview = () => false;
  export let openFunctionCatalog = () => {};
  export let openAISettings = () => {};
  export let checkForUpdates = () => {};
  export let openUpdatePage = () => {};
  export let createFolder = () => {};
  export let renameFolder = () => {};
  export let deleteFolder = () => {};
  export let onError = () => {};
  export let onAggDownloadInfo = () => {};
</script>

<div class="main-content">
  {#if currentPage === "terminals"}
    <TerminalsPage
      {availableTerminalTypes}
      bind:selectedTerminalType
      {aiSettings}
      {visibleTerminalCount}
      {errorMessage}
      {layoutTree}
      {terminalMap}
      {activeTerminalId}
      {notesPanelOpen}
      {notesPanelWidth}
      {terminals}
      {showAIMenu}
      {persistDefaultTerminalType}
      {addTerminal}
      {toggleAIMenu}
      {openAIPanel}
      dismissError={() => onError('')}
      setActiveTerminalId={(id) => { activeTerminalId = id; }}
      {splitTerminal}
      {toggleMinimize}
      {removeTerminal}
      {reconnectSSH}
      {startEditingName}
      {saveTerminalName}
      {handleResize}
      touchLayoutTree={() => { layoutTree = layoutTree; }}
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
      {startNotesDrag}
      closeNotesPanel={() => { notesPanelOpen = false; setTimeout(handleResize, 50); }}
    />
  {:else if currentPage === "templateManager"}
    <TemplateManager
      {SaveTemplate}
      {UpdateTemplate}
      {DeleteTemplate}
      {ToggleFavorite}
      {templateToEdit}
      {templates}
      on:templateUpdated={(event) => {
        if (event.detail && Array.isArray(event.detail)) {
          templates = normalizeTemplates(event.detail);
        } else {
          GetTemplates().then(result => {
            templates = normalizeTemplates(result);
          });
        }
      }}
      on:editTemplate={(event) => { templateToEdit = event.detail; openPage("templateManager"); }}
      on:backToTerminals={() => { templateToEdit = null; openPage("terminals"); }}
    />
  {:else if currentPage === "fileBrowser"}
    <FileBrowser
      remoteTerminalId={fileBrowserRemoteTerminalId}
      remoteLabel={fileBrowserRemoteLabel}
      on:openTerminalHere={(e) => addTerminal(selectedTerminalType, `Terminal in ${e.detail}`, false, e.detail)}
      on:insertIntoTerminal={insertFileIntoActiveTerminal}
      on:openInNotes={handleOpenInNotes}
      on:switchToLocal={() => { fileBrowserRemoteTerminalId = 0; fileBrowserRemoteLabel = ''; }}
      on:remoteCD={(e) => { WriteToTerminal(e.detail.terminalId, `cd ${shellQuotePath(e.detail.path)}\r`); }}
    />
  {:else if currentPage === "workflowBuilder"}
    <WorkflowBuilder
      queuedCatalogEntry={workflowBuilderQueuedEntry}
      queuedCatalogEntries={workflowBuilderQueuedEntries}
      activeTerminalId={activeTerminalId}
      activeTerminalType={terminals.find((terminal) => terminal.id === activeTerminalId)?.type || selectedTerminalType}
      activeTerminalName={terminals.find((terminal) => terminal.id === activeTerminalId)?.name || ''}
      activeTerminalOutput={terminals.find((terminal) => terminal.id === activeTerminalId)?.outputBuffer || ''}
      on:backToTerminals={() => { openPage("terminals"); }}
    />
  {:else if currentPage === "aiHub"}
    <AIHubView
      settings={aiSettings}
      flow={aiToolFlowConfig}
      promptIntroPreview={getEditablePromptIntroPreview()}
      usingDefaultIntro={isUsingDefaultPromptIntroPreview()}
      onOpenCatalog={openFunctionCatalog}
      onOpenSettings={openAISettings}
      onOpenLogs={() => openPage("activityLogs")}
    />
  {:else if currentPage === "historyDashboard"}
    <HistoryDashboard
      activeTerminalId={activeTerminalId}
      on:backToTerminals={() => { openPage("terminals"); }}
      on:runcommand={(e) => { WriteToTerminal(e.detail.terminalId, e.detail.command + '\r'); openPage("terminals"); }}
    />
  {:else if currentPage === "activityLogs"}
    <ActivityLogViewer
      on:backToTerminals={() => { openPage("terminals"); }}
    />
  {:else if currentPage === "recordings"}
    <RecordingPlayer
      recordings={recordingList}
      {aggAvailable}
      loadRecording={GetRecording}
      on:delete={async (e) => {
        try {
          await DeleteRecording(e.detail);
          recordingList = await ListRecordings();
        } catch (err) { console.error('Delete recording failed:', err); }
      }}
      on:exportscrubbed={async (e) => {
        try {
          await ExportRecordingScrubbed(e.detail);
        } catch (err) { console.error('Export scrubbed failed:', err); }
      }}
      on:exportgif={async (e) => {
        try {
          await ExportRecordingGIF(e.detail);
        } catch (err) { console.error('GIF export failed:', err); }
      }}
      on:exporttrimmed={async (e) => {
        try {
          await ExportRecordingTrimmed(e.detail.id, JSON.stringify(e.detail.cuts));
        } catch (err) { console.error('Trimmed export failed:', err); }
      }}
      on:exporttrimmedgif={async (e) => {
        try {
          await ExportRecordingTrimmedGIF(e.detail.id, JSON.stringify(e.detail.cuts));
        } catch (err) { console.error('Trimmed GIF export failed:', err); }
      }}
    />
  {:else if currentPage === "settings"}
    <SettingsView
      {notesPanelOpen}
      bind:showFolderManager
      {historyTrackingEnabled}
      {aggAvailable}
      {updateChecking}
      {updateInfo}
      {customFolders}
      bind:newFolderName
      onOpenAISettings={openAISettings}
      onManageTemplates={() => { templateToEdit = null; openPage("templateManager"); }}
      onToggleNotes={() => { notesPanelOpen = !notesPanelOpen; openPage("terminals"); setTimeout(handleResize, 50); }}
      onToggleHistory={async () => { const next = !historyTrackingEnabled; await SetHistoryTracking(next); historyTrackingEnabled = next; }}
      onInstallAgg={async () => { if (aggAvailable) return; try { onAggDownloadInfo(await GetAggDownloadInfo()); } catch (err) { onError(`Failed to get agg info: ${err.message || err}`); } }}
      onCheckUpdates={checkForUpdates}
      onOpenUpdatePage={openUpdatePage}
      onCreateFolder={createFolder}
      onRenameFolder={renameFolder}
      onDeleteFolder={deleteFolder}
    />
  {/if}
</div>
