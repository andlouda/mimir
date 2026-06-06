<script>
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

  export let showTemplatePicker = false;
  export let templates = [];
  export let applyTemplate = () => {};
  export let templatePromptState = null;
  export let closeTemplatePrompt = () => {};
  export let submitTemplatePrompt = () => {};
  export let handleTemplatePromptFieldChange = () => {};
  export let aiPanelState = null;
  export let terminals = [];
  export let closeAIPanel = () => {};
  export let onError = () => {};
  export let showAISettings = false;
  export let aiSettings = {};
  export let aiToolFlowConfig = {};
  export let aiToolFlowLists = {};
  export let aiProviders = [];
  export let promptPreview = '';
  export let closeAISettings = () => {};
  export let saveAISettings = () => {};
  export let applyAISettingsDefaults = () => {};
  export let setDevOpsPrePromptExample = () => {};
  export let showFunctionCatalog = false;
  export let functionCatalog = [];
  export let discoveryTerminalType = '';
  export let queueWorkflowFromCatalog = () => {};
  export let closeFunctionCatalog = () => {};
  export let showSSHProfileModal = false;
  export let sshProfiles = [];
  export let sshSecretBackend = '';
  export let sshConnecting = false;
  export let connectSSHProfile = () => {};
  export let closeSSHProfileModal = () => {};
  export let onProfilesChanged = () => {};
  export let hostKeyVerifyState = null;
  export let acceptHostKey = () => {};
  export let rejectHostKey = () => {};
  export let showWorkflowPicker = false;
  export let workflowPickerPlaybooks = [];
  export let workflowPickerLoading = false;
  export let runWorkflowFromPicker = () => {};
  export let closeWorkflowPicker = () => {};
  export let aggDownloadInfo = null;
  export let downloadingAgg = false;
  export let cancelAggDownload = () => {};
  export let runAggDownload = () => {};
  export let transcriptViewerState = null;
  export let closeTranscriptViewer = () => {};
</script>

{#if showTemplatePicker}
  <TemplatePicker
    templates={templates}
    onSelect={(name) => { showTemplatePicker = false; applyTemplate(name); }}
    onClose={() => { showTemplatePicker = false; }}
  />
{/if}

{#if showWorkflowPicker}
  <WorkflowPicker
    playbooks={workflowPickerPlaybooks}
    loading={workflowPickerLoading}
    onSelect={(pb) => { showWorkflowPicker = false; runWorkflowFromPicker(pb); }}
    onClose={closeWorkflowPicker}
  />
{/if}

{#if templatePromptState}
  <TemplatePromptModal
    bind:state={templatePromptState}
    onClose={closeTemplatePrompt}
    onSubmit={submitTemplatePrompt}
    onFieldChange={handleTemplatePromptFieldChange}
  />
{/if}

{#if aiPanelState}
  <AIPanelModal
    bind:state={aiPanelState}
    terminalOutput={terminals.find(t => t.id === aiPanelState.terminalId)?.outputBuffer || ''}
    onClose={closeAIPanel}
    onError={onError}
  />
{/if}

{#if showAISettings}
  <AISettingsModal
    bind:settings={aiSettings}
    bind:flow={aiToolFlowConfig}
    bind:lists={aiToolFlowLists}
    providers={aiProviders}
    {promptPreview}
    onClose={closeAISettings}
    onSave={saveAISettings}
    onProviderChange={applyAISettingsDefaults}
    onUseDevOpsExample={setDevOpsPrePromptExample}
  />
{/if}

{#if showFunctionCatalog}
  <FunctionCatalogModal
    catalog={functionCatalog}
    {discoveryTerminalType}
    onClose={closeFunctionCatalog}
    onAddToWorkflow={queueWorkflowFromCatalog}
    onError={onError}
  />
{/if}

{#if showSSHProfileModal}
  <SSHProfileModal
    profiles={sshProfiles}
    secretBackend={sshSecretBackend}
    connecting={sshConnecting}
    onClose={closeSSHProfileModal}
    onConnect={connectSSHProfile}
    onProfilesChanged={onProfilesChanged}
    onError={onError}
  />
{/if}

{#if hostKeyVerifyState}
  <HostKeyModal state={hostKeyVerifyState} onAccept={acceptHostKey} onReject={rejectHostKey} />
{/if}

{#if aggDownloadInfo}
  <AggDownloadModal
    info={aggDownloadInfo}
    downloading={downloadingAgg}
    onCancel={cancelAggDownload}
    onDownload={runAggDownload}
  />
{/if}

{#if transcriptViewerState}
  <TranscriptViewerModal
    initialResumeId={transcriptViewerState.resumeId}
    initialLabel={transcriptViewerState.label}
    onClose={closeTranscriptViewer}
    onError={onError}
  />
{/if}
