<script>
  // Every modal's open/closed state and content lives in a store; the
  // handlers are actions. Nothing is threaded through App.svelte.
  import { getTerminalOutput } from './terminals/outputTail.js';
  import TemplatePicker from './modals/TemplatePicker.svelte';
  import WorkflowPicker from './modals/WorkflowPicker.svelte';
  import TemplatePromptModal from './modals/TemplatePromptModal.svelte';
  import AIPanelModal from './modals/AIPanelModal.svelte';
  import AISettingsModal from './modals/AISettingsModal.svelte';
  import FunctionCatalogModal from './modals/FunctionCatalogModal.svelte';
  import SSHProfileModal from './modals/SSHProfileModal.svelte';
  import HostKeyModal from './modals/HostKeyModal.svelte';
  import AggDownloadModal from './modals/AggDownloadModal.svelte';
  import TranscriptViewerModal from './modals/TranscriptViewerModal.svelte';
  import DotEnvViewerModal from './modals/DotEnvViewerModal.svelte';
  import { aiPanelState, aiProviders, aiSettings, aiToolFlowConfig, aiToolFlowLists, functionCatalog, showAISettings, showFunctionCatalog } from './stores/aiStore.js';
  import { aggDownloadInfo, downloadingAgg } from './stores/sessionStore.js';
  import { hostKeyVerifyState, showSSHProfileModal, sshConnecting, sshProfiles, sshSecretBackend } from './stores/sshStore.js';
  import { showTemplatePicker, showWorkflowPicker, templatePromptState, templates, workflowPickerLoading, workflowPickerPlaybooks } from './stores/templateStore.js';
  import { activeTerminalId, discoveryTerminalType } from './stores/terminalStore.js';
  import { dotEnvViewerState, errorMessage, transcriptViewerState } from './stores/uiStore.js';
  import { runAggDownload } from './actions/aggActions.js';
  import { applyAISettingsDefaults, clearAIApiKey, closeAISettings, getAIToolPromptPreview, saveAISettings, setDevOpsPrePromptExample } from './actions/aiSettingsActions.js';
  import { queueWorkflowFromCatalog } from './actions/navigationActions.js';
  import { persistTerminalState } from './actions/sessionActions.js';
  import { createSSHActions, loadSSHProfiles } from './actions/sshActions.js';
  import { applyTemplate, closeTemplatePrompt, handleTemplatePromptFieldChange, runWorkflowFromPicker, submitTemplatePrompt } from './actions/templateActions.js';
  import { createTerminalInstance, reinitializeTerminals } from './actions/terminalActions.js';

  const { connectSSHProfile, acceptHostKey, rejectHostKey } = createSSHActions({ createTerminalInstance, persistTerminalState, reinitializeTerminals });
  const promptPreview = getAIToolPromptPreview();
  const reportError = (msg) => errorMessage.set(msg);
  const closeWorkflowPicker = () => showWorkflowPicker.set(false);
  const closeAIPanel = () => aiPanelState.set(null);
  const closeFunctionCatalog = () => showFunctionCatalog.set(false);
  const closeSSHProfileModal = () => showSSHProfileModal.set(false);
  const cancelAggDownload = () => aggDownloadInfo.set(null);
  const closeTranscriptViewer = () => transcriptViewerState.set(null);
  const closeDotEnvViewer = () => dotEnvViewerState.set(null);
  async function onProfilesChanged(p) {
    sshProfiles.set(Array.isArray(p) ? p : []);
    await loadSSHProfiles();
  }
</script>

{#if $showTemplatePicker}
  <TemplatePicker
    templates={$templates}
    onSelect={(name) => { $showTemplatePicker = false; applyTemplate(name); }}
    onClose={() => { $showTemplatePicker = false; }}
  />
{/if}

{#if $showWorkflowPicker}
  <WorkflowPicker
    playbooks={$workflowPickerPlaybooks}
    loading={$workflowPickerLoading}
    onSelect={(pb) => { $showWorkflowPicker = false; runWorkflowFromPicker(pb); }}
    onClose={closeWorkflowPicker}
  />
{/if}

{#if $templatePromptState}
  <TemplatePromptModal
    bind:state={$templatePromptState}
    onClose={closeTemplatePrompt}
    onSubmit={submitTemplatePrompt}
    onFieldChange={handleTemplatePromptFieldChange}
  />
{/if}

{#if $aiPanelState}
  <AIPanelModal
    bind:state={$aiPanelState}
    terminalOutput={getTerminalOutput($aiPanelState.terminalId)}
    onClose={closeAIPanel}
    onError={reportError}
  />
{/if}

{#if $showAISettings}
  <AISettingsModal
    bind:settings={$aiSettings}
    bind:flow={$aiToolFlowConfig}
    bind:lists={$aiToolFlowLists}
    providers={$aiProviders}
    {promptPreview}
    onClose={closeAISettings}
    onSave={saveAISettings}
    onProviderChange={applyAISettingsDefaults}
    onUseDevOpsExample={setDevOpsPrePromptExample}
    onClearApiKey={clearAIApiKey}
  />
{/if}

{#if $showFunctionCatalog}
  <FunctionCatalogModal
    catalog={$functionCatalog}
    discoveryTerminalType={$discoveryTerminalType}
    discoveryTerminalId={$activeTerminalId}
    onClose={closeFunctionCatalog}
    onAddToWorkflow={queueWorkflowFromCatalog}
    onError={reportError}
  />
{/if}

{#if $showSSHProfileModal}
  <SSHProfileModal
    profiles={$sshProfiles}
    secretBackend={$sshSecretBackend}
    connecting={$sshConnecting}
    onClose={closeSSHProfileModal}
    onConnect={connectSSHProfile}
    onProfilesChanged={onProfilesChanged}
    onError={reportError}
  />
{/if}

{#if $hostKeyVerifyState}
  <HostKeyModal state={$hostKeyVerifyState} onAccept={acceptHostKey} onReject={rejectHostKey} />
{/if}

{#if $aggDownloadInfo}
  <AggDownloadModal
    info={$aggDownloadInfo}
    downloading={$downloadingAgg}
    onCancel={cancelAggDownload}
    onDownload={runAggDownload}
  />
{/if}

{#if $transcriptViewerState}
  <TranscriptViewerModal
    initialResumeId={$transcriptViewerState.resumeId}
    initialLabel={$transcriptViewerState.label}
    onClose={closeTranscriptViewer}
    onError={reportError}
  />
{/if}

{#if $dotEnvViewerState}
  <DotEnvViewerModal
    terminalId={$dotEnvViewerState.terminalId}
    terminalType={$dotEnvViewerState.terminalType}
    label={$dotEnvViewerState.label}
    onClose={closeDotEnvViewer}
  />
{/if}
