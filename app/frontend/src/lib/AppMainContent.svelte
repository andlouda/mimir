<script>
  import { getTerminalOutput } from './terminals/outputTail.js';
  import { SaveTemplate, UpdateTemplate, DeleteTemplate, ToggleFavorite, GetTemplates, WriteToTerminal, GetRecording, DeleteRecording, ExportRecordingScrubbed, ExportRecordingGIF, ExportRecordingTrimmed, ExportRecordingTrimmedGIF, ListRecordings } from './backend.js';
  import TemplateManager from './TemplateManager.svelte';
  import FileBrowser from './FileBrowser.svelte';
  import WorkflowBuilder from './workflows/WorkflowBuilder.svelte';
  import ActivityLogViewer from './ActivityLogViewer.svelte';
  import HistoryDashboard from './HistoryDashboard.svelte';
  import RecordingPlayer from './RecordingPlayer.svelte';
  import TerminalsPage from './views/TerminalsPage.svelte';
  import AIHubView from './views/AIHubView.svelte';
  import SettingsView from './views/SettingsView.svelte';
  import AgentWorkspaceView from './views/AgentWorkspaceView.svelte';
  import { normalizeTemplates } from './templates/templateHelpers';
  import { containsControlChars, shellQuotePath } from './util';
  import { errorMessage as errorMessageStore } from './stores/uiStore.js';
  import { t as tr } from './i18n.js';
  import { aiSettings } from './stores/aiStore.js';
  import { activeTerminalId, selectedTerminalType, terminals } from './stores/terminalStore.js';
  import { addTerminal } from './actions/terminalActions.js';

  export let currentPage = 'terminals';
  export let templateToEdit = null;
  export let templates = [];
  export let fileBrowserRemoteTerminalId = 0;
  export let fileBrowserRemoteLabel = '';
  export let workflowBuilderQueuedEntry = null;
  export let workflowBuilderQueuedEntries = [];
  export let aiToolFlowConfig = {};
  export let recordingList = [];
  export let aggAvailable = false;

  // The path comes from the remote SFTP listing, i.e. the remote server picks
  // the bytes. shellQuotePath handles quotes, but control characters would
  // still be typed into the PTY (CR submits early, ESC can hit readline
  // bindings), so those paths are refused outright.
  function handleRemoteCD(terminalId, path) {
    if (containsControlChars(path)) {
      errorMessageStore.set('Refused to cd: remote path contains control characters.');
      return;
    }
    WriteToTerminal(terminalId, `cd ${shellQuotePath(path)}\r`);
  }

  export let openPage = () => {};
  export let selectTerminal = () => {};
  export let insertFileIntoActiveTerminal = () => {};
  export let handleOpenInNotes = () => {};
  export let getEditablePromptIntroPreview = () => '';
  export let isUsingDefaultPromptIntroPreview = () => false;
  export let openFunctionCatalog = () => {};
  export let openAISettings = () => {};
  export let onError = () => {};
</script>

<div class="main-content">
  {#if currentPage === "terminals"}
    <TerminalsPage />
  {:else if currentPage === "agentWorkspace"}
    <AgentWorkspaceView {selectTerminal} />
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
      on:openTerminalHere={(e) => addTerminal($selectedTerminalType, `Terminal in ${e.detail}`, false, e.detail)}
      on:insertIntoTerminal={insertFileIntoActiveTerminal}
      on:openInNotes={handleOpenInNotes}
      on:switchToLocal={() => { fileBrowserRemoteTerminalId = 0; fileBrowserRemoteLabel = ''; }}
      on:remoteCD={(e) => handleRemoteCD(e.detail.terminalId, e.detail.path)}
    />
  {:else if currentPage === "workflowBuilder"}
    <WorkflowBuilder
      queuedCatalogEntry={workflowBuilderQueuedEntry}
      queuedCatalogEntries={workflowBuilderQueuedEntries}
      $activeTerminalId={$activeTerminalId}
      activeTerminalType={$terminals.find((terminal) => terminal.id === $activeTerminalId)?.type || $selectedTerminalType}
      activeTerminalName={$terminals.find((terminal) => terminal.id === $activeTerminalId)?.name || ''}
      activeTerminalOutput={getTerminalOutput($activeTerminalId)}
      on:backToTerminals={() => { openPage("terminals"); }}
    />
  {:else if currentPage === "aiHub"}
    <AIHubView
      settings={$aiSettings}
      flow={aiToolFlowConfig}
      promptIntroPreview={getEditablePromptIntroPreview()}
      usingDefaultIntro={isUsingDefaultPromptIntroPreview()}
      onOpenCatalog={openFunctionCatalog}
      onOpenSettings={openAISettings}
      onOpenLogs={() => openPage("activityLogs")}
    />
  {:else if currentPage === "historyDashboard"}
    <HistoryDashboard
      $activeTerminalId={$activeTerminalId}
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
        } catch (err) { onError($tr('appMessages.deleteRecordingFailed', { error: err.message || err })); }
      }}
      on:exportscrubbed={async (e) => {
        try {
          const path = await ExportRecordingScrubbed(e.detail);
          if (path) onError($tr('appMessages.exportSaved', { path }));
        } catch (err) { onError($tr('appMessages.exportScrubbedFailed', { error: err.message || err })); }
      }}
      on:exportgif={async (e) => {
        const id = typeof e.detail === 'object' ? e.detail.id : e.detail;
        try {
          const path = await ExportRecordingGIF(id);
          if (path) onError($tr('appMessages.gifExportSaved', { path }));
        } catch (err) { onError($tr('appMessages.gifExportFailed', { error: err.message || err })); }
        finally { e.detail?.done?.(); }
      }}
      on:exporttrimmed={async (e) => {
        try {
          const path = await ExportRecordingTrimmed(e.detail.id, JSON.stringify(e.detail.cuts));
          if (path) onError($tr('appMessages.trimmedExportSaved', { path }));
        } catch (err) { onError($tr('appMessages.trimmedExportFailed', { error: err.message || err })); }
      }}
      on:exporttrimmedgif={async (e) => {
        try {
          const path = await ExportRecordingTrimmedGIF(e.detail.id, JSON.stringify(e.detail.cuts));
          if (path) onError($tr('appMessages.trimmedGifSaved', { path }));
        } catch (err) { onError($tr('appMessages.trimmedGifFailed', { error: err.message || err })); }
      }}
    />
  {:else if currentPage === "settings"}
    <SettingsView />
  {/if}

<!-- The error/result banner belongs to the app, not to the Terminals page: export results and
     failures used to be invisible on the Recordings and Settings pages. -->
{#if $errorMessageStore}
  <div class="error-message">
    <span class="error-icon">!</span>
    <span class="error-text">{$errorMessageStore}</span>
    <button class="error-dismiss" on:click={() => onError('')}>×</button>
  </div>
{/if}

</div>
