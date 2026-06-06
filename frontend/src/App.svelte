<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { t as tr } from './lib/i18n.js';
  import { StartTerminal, WriteToTerminal, ResizeTerminal, CloseTerminal, InitializeTerminal, ConfirmFrontendReady, ApplyTemplate, GetTemplates, GetLoadedSessionData, UpdateTerminalState, RemoveTerminalState, SaveCurrentSession, GetSSHProfiles, GetSSHSecretBackend, StartSSHTerminal, ReconnectSSHTerminal, CloseSSHTerminalFull, AcceptSSHHostKey, RejectSSHHostKey, GetSSHTerminalLabel, KillTmuxSession, StartRecording, StopRecording, ListRecordings, IsAggInstalled, DownloadAgg, GetTerminalFolders, SaveTerminalFolder, UpdateTerminalFolder, DeleteTerminalFolder, IsHistoryTrackingEnabled, SetHistoryTracking } from '../wailsjs/go/main/App';
  import { EventsOn } from '../wailsjs/runtime';
  import { Terminal } from '@xterm/xterm';
  import { FitAddon } from '@xterm/addon-fit';
  import { SearchAddon } from '@xterm/addon-search';
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
  import { replaceLeaf, removeLeafFromTree, collectLeafIds, swapLeaves } from './lib/terminals/layoutTree';
  import { terminalGroupFor, terminalGroupLabel, groupedSidebarTerminals } from './lib/terminals/sidebarGroups';
  import { normalizeAIToolFlowConfig, listToText, textToList } from './lib/ai/configHelpers';
  import { normalizeTemplates, extractTemplateVariables, buildPromptLabel } from './lib/templates/templateHelpers';
  import { shellQuotePath, generateResumeId, dedupeSavedSessionTerminals, sanitizeTranscriptPreview } from './lib/util';
  import { generateTmuxSessionName } from './lib/terminals/tmuxLifecycle';
  import { safelyWriteTerminal, safelyFitAndResizeTerminal, safelyAttachTerminal, safelyDisposeTerminal } from './lib/terminals/xtermLifecycle';

  const isWindowsPlatform = typeof navigator !== 'undefined' && navigator.userAgent.includes('Windows');
  const tmuxCapableTerminalTypes = new Set(['bash', 'zsh', 'wsl']);
  const defaultTerminalTypeStorageKey = 'mimir-default-terminal-type';

  function preferredTerminalType(options = availableTerminalTypes) {
    const order = isWindowsPlatform ? ['wsl', 'bash', 'powershell', 'cmd'] : ['bash', 'zsh'];
    return order.map((value) => options.find((option) => option.value === value)).find(Boolean) || options[0];
  }

  function readDefaultTerminalType() {
    try {
      const saved = localStorage.getItem(defaultTerminalTypeStorageKey);
      if (saved) return saved;
    } catch (error) {
      console.error('Failed to read default terminal type:', error);
    }
    return preferredTerminalType()?.value || (isWindowsPlatform ? 'wsl' : 'bash');
  }

  function persistDefaultTerminalType() {
    try {
      localStorage.setItem(defaultTerminalTypeStorageKey, selectedTerminalType);
    } catch (error) {
      console.error('Failed to save default terminal type:', error);
    }
  }

  function formatErrorMessage(error, fallback = 'An unknown error occurred.') {
    if (error instanceof Error && error.message) return error.message;
    if (typeof error === 'string' && error.trim()) return error;
    if (error && typeof error.message === 'string' && error.message.trim()) return error.message;
    try {
      const serialized = JSON.stringify(error);
      if (serialized && serialized !== 'null' && serialized !== '{}') return serialized;
    } catch {
      // Ignore serialization failures and use the fallback.
    }
    return fallback;
  }

  function isAuditPage(page = currentPage) {
    return ['historyDashboard', 'activityLogs', 'recordings'].includes(page);
  }

  // terminalGroupFor / terminalGroupLabel / groupedSidebarTerminals live in
  // ./lib/terminals/sidebarGroups.js (pure, state-free helpers).

  function toggleTerminalFolder(groupID) {
    terminalSessionFoldersOpen = {
      ...terminalSessionFoldersOpen,
      [groupID]: !(terminalSessionFoldersOpen[groupID] ?? true),
    };
  }

  async function loadCustomFolders() {
    try {
      customFolders = await GetTerminalFolders();
    } catch (e) {
      console.error('Failed to load terminal folders:', e);
    }
  }

  async function createFolder() {
    const name = newFolderName.trim();
    if (!name) return;
    try {
      customFolders = await SaveTerminalFolder(JSON.stringify({ name, position: customFolders.length + 1 }));
      newFolderName = '';
    } catch (e) {
      errorMessage = `Failed to create folder: ${e.message || e}`;
    }
  }

  async function renameFolder(folder, newName) {
    const trimmed = newName.trim();
    if (!trimmed || trimmed === folder.name) return;
    try {
      customFolders = await UpdateTerminalFolder(JSON.stringify({ ...folder, name: trimmed }));
    } catch (e) {
      errorMessage = `Failed to rename folder: ${e.message || e}`;
    }
  }

  async function deleteFolder(folderId) {
    try {
      customFolders = await DeleteTerminalFolder(folderId);
      // Reset all terminals assigned to this folder back to auto-grouping
      const affected = terminals.filter((t) => t.folderId === folderId);
      terminals = terminals.map((t) => t.folderId === folderId ? { ...t, folderId: '' } : t);
      for (const t of affected) {
        persistTerminalState({ ...t, folderId: '' });
      }
    } catch (e) {
      errorMessage = `Failed to delete folder: ${e.message || e}`;
    }
  }

  function assignTerminalToFolder(terminalId, folderId) {
    terminals = terminals.map((t) => t.id === terminalId ? { ...t, folderId } : t);
    const term = terminals.find((t) => t.id === terminalId);
    if (term) persistTerminalState(term);
  }

  function selectSidebarTerminal(term) {
    if (!term) return;
    if (term.minimized) {
      toggleMinimize(term.id);
    }
    activeTerminalId = term.id;
    openPage('terminals');
    setTimeout(handleResize, 50);
  }

  function openTerminals() {
    templateToEdit = null;
    openPage('terminals');
  }

  async function startTerminalBackend(type, tmuxSessionName = '') {
    const startWithOptions = window['go']?.['main']?.['App']?.['StartTerminalWithOptions'];
    if (typeof startWithOptions === 'function') {
      return startWithOptions(type, tmuxSessionName);
    }
    return StartTerminal(type);
  }

  async function readTmuxStatus(id) {
    const getStatus = window['go']?.['main']?.['App']?.['GetTerminalTmuxStatus'];
    if (typeof getStatus === 'function') {
      return getStatus(id);
    }
    const getSSHStatus = window['go']?.['main']?.['App']?.['GetSSHTerminalTmuxStatus'];
    if (typeof getSSHStatus === 'function') {
      return getSSHStatus(id);
    }
    return { active: false, sessionName: '' };
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

  let terminals = [];
  let errorMessage = "";
  let draggedTerminalId = null;
  let selectedTerminalType = readDefaultTerminalType();
  let activeTerminalId = null;
  let templates = [];
  let searchQuery = '';
  let currentPage = "terminals";
  let recordingList = [];
  let aggAvailable = false;
  let aggStatus = 'missing';
  let downloadingAgg = false;
  let aggDownloadInfo = null; // {url, destination, platform} — shown in confirm dialog
  let templateToEdit = null;
  let layoutTree = null;
  let templatePromptState = null;
  let aiPanelState = null;
  let showFunctionCatalog = false;
  let functionCatalog = [];
  let workflowBuilderQueuedEntry = null;
  let workflowBuilderQueuedEntries = [];
  let showAISettings = false;
  let showAIMenu = false;
  let aiProviders = [];
  let aiSettings = {
    provider: 'openai',
    model: 'gpt-5.4-mini',
    baseUrl: '',
    apiKey: '',
  };
  let aiToolFlowConfig = {
    prompt: {
      prePrompt: '',
      requireStableToolId: true,
      includeRisk: true,
      includeCategory: true,
      includeTerminalOutput: true,
      maxTerminalContext: 12000,
      allowTemplateNameFallback: true,
    },
    toolFilter: {
      includeCategories: [],
      excludeCategories: [],
      includeToolIds: [],
      excludeToolIds: [],
    },
    approval: {
      respectStepFlag: true,
      requireApprovalForLow: false,
      requireApprovalForMedium: true,
      requireApprovalForHigh: true,
    },
    execution: {
      enabled: true,
      workflowMode: 'approve',
      workflowIdPrefix: 'ai-tool-run',
      workflowName: 'AI Tool Run',
      forceRequiresApproval: false,
    },
  };
  const immutableAIToolGuardrails = [
    'System guardrails (non-editable):',
    '1. Use only registered tool IDs from the provided list.',
    '2. Never invent raw shell commands and never use name-based fallback.',
    '3. Only choose read-only, diagnostic tools.',
    '4. Never choose tools that write, delete, install, restart, stop, kill, deploy, patch, scale, prune, or otherwise mutate files, packages, services, containers, clusters, firewall rules, or system state.',
    '5. Never access, print, or exfiltrate secrets, tokens, API keys, SSH keys, environment credentials, or credential stores.',
    '6. If the request requires mutation, secret access, or anything ambiguous, return an empty toolId and explain the refusal.'
  ].join('\n');
  const devopsPrePromptExample = 'You are a cautious DevOps terminal assistant. Prefer read-only diagnostics first, summarize likely impact before suggesting changes, and choose the least disruptive tool that helps inspect Kubernetes, Docker, networking, logs, processes, and system health. Escalate to mutating actions only when the user clearly asks for them.';
  let aiToolFlowLists = {
    includeCategories: '',
    excludeCategories: '',
    includeToolIds: '',
    excludeToolIds: '',
  };

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

  // ─── SSH State ─────────────────────────────────────────
  let sshProfiles = [];
  let showSSHProfileModal = false;
  let showTemplatePicker = false;
  let showWorkflowPicker = false;
  let workflowPickerPlaybooks = [];
  let workflowPickerLoading = false;
  let transcriptViewerState = null; // { resumeId, label } when modal is open
  let sshSecretBackend = '';
  let sshConnecting = false;
  let hostKeyVerifyState = null; // { status, host, fingerprint, keyType, message, profileID, profile }

  // ─── SFTP / Remote File Browser State ───────────────────
  let fileBrowserRemoteTerminalId = 0;
  let fileBrowserRemoteLabel = '';
  let terminalSessionFoldersOpen = { local: true, ssh: true, windows: true, other: true };
  let customFolders = [];
  let showFolderManager = false;
  let newFolderName = '';
  let historyTrackingEnabled = false;
  let historyConsentDismissed = false;
  let updateInfo = null;
  let updateChecking = false;
  let updateDownloading = false;
  let updateProgress = null;
  let updateInstalled = false;
  let notesPanelOpen = false;
  let notesPanelWidth = parseInt(localStorage.getItem('mimir-notes-width') || '380');
  let sessionSaveTimer = null;

  function scheduleSessionSave(delayMs = 250) {
    if (sessionSaveTimer) {
      clearTimeout(sessionSaveTimer);
    }
    sessionSaveTimer = setTimeout(() => {
      SaveCurrentSession().catch((error) => {
        console.error('Failed to save current session:', error);
      });
      sessionSaveTimer = null;
    }, delayMs);
  }

  function persistTerminalState(term, overrides = {}) {
    const next = { ...term, ...overrides };
    window['go']['main']['App']['UpdateTerminalState'](
      next.id,
      next.type,
      next.name,
      next.minimized,
      next.sshProfileId || '',
      next.tmuxSessionName || '',
      next.resumeId || '',
      next.restoreClass || 'fresh',
      next.folderId || ''
    );
    scheduleSessionSave();
  }

  async function loadTranscriptExcerpt(resumeId, maxBytes = 8000) {
    if (!resumeId) return '';
    try {
      const raw = await window['go']['main']['App']['GetTerminalTranscriptExcerpt'](resumeId, maxBytes);
      return sanitizeTranscriptPreview(raw);
    } catch (error) {
      console.error(`Failed to load transcript excerpt for ${resumeId}:`, error);
      return '';
    }
  }

  async function loadSSHProfiles() {
    try {
      sshProfiles = await GetSSHProfiles();
      sshSecretBackend = await GetSSHSecretBackend();
    } catch (e) {
      console.error('Failed to load SSH profiles:', e);
    }
  }

  function openSSHProfilePicker() {
    loadSSHProfiles();
    showSSHProfileModal = true;
  }

  async function checkForUpdates() {
    updateChecking = true;
    try {
      const raw = await window['go']['main']['App']['CheckForUpdates']();
      updateInfo = JSON.parse(raw);
    } catch (error) {
      updateInfo = { error: error.message || String(error), configured: false };
    } finally {
      updateChecking = false;
    }
  }

  async function openUpdatePage() {
    try {
      await window['go']['main']['App']['OpenUpdatePage'](updateInfo?.releaseUrl || '');
    } catch (error) {
      errorMessage = `Update-Seite konnte nicht geoeffnet werden: ${error.message || error}`;
    }
  }

  async function downloadUpdate() {
    updateDownloading = true;
    updateProgress = { stage: 'downloading', percent: 0 };
    try {
      const raw = await window['go']['main']['App']['StartUpdateDownload']();
      const result = JSON.parse(raw);
      if (result.error) {
        errorMessage = `Update failed: ${result.error}`;
        updateProgress = null;
        updateDownloading = false;
      }
    } catch (error) {
      errorMessage = `Update failed: ${error.message || error}`;
      updateProgress = null;
      updateDownloading = false;
    }
  }

  async function restartApp() {
    try {
      await window['go']['main']['App']['RestartApp']();
    } catch (err) {
      errorMessage = `Restart failed: ${err.message || err}`;
    }
  }

  async function runAggDownload() {
    downloadingAgg = true;
    try {
      await DownloadAgg();
      aggAvailable = await IsAggInstalled().catch(() => false);
      aggStatus = await window['go']['main']['App']['GetAggStatus']().catch(() => 'missing');
      aggDownloadInfo = null;
      if (!aggAvailable && aggStatus === 'incompatible') {
        errorMessage = 'agg was downloaded but is incompatible with your system. Install agg via your package manager (e.g. cargo install agg).';
      }
    } catch (err) {
      errorMessage = `agg Download fehlgeschlagen: ${err.message || err}`;
    } finally {
      downloadingAgg = false;
    }
  }

  async function connectSSHProfile(profile) {
    try {
      sshConnecting = true;
      const id = await StartSSHTerminal(profile.id);
      if (!id) {
        errorMessage = 'Failed to start SSH terminal. The backend returned an invalid ID.';
        sshConnecting = false;
        return;
      }
      showSSHProfileModal = false;
      sshConnecting = false;

      const name = `SSH: ${profile.name}`;
      const newLeaf = { type: 'leaf', terminalId: id };
      if (layoutTree === null) {
        layoutTree = newLeaf;
      } else {
        layoutTree = {
          type: 'split',
          direction: 'horizontal',
          ratio: 0.5,
          children: [layoutTree, newLeaf]
        };
      }

      const newTerminal = await createTerminalInstance(id, 'ssh', name, false, profile.id, false, '', '', 'fresh');
      activeTerminalId = id;
      persistTerminalState({ ...newTerminal, type: 'ssh', minimized: false, sshProfileId: profile.id });
      await reinitializeTerminals();
    } catch (e) {
      sshConnecting = false;
      const errMsg = e.message || String(e);
      if (errMsg.includes('HOST_KEY_VERIFY|')) {
        // Parse: HOST_KEY_VERIFY|status|host|fingerprint|keyType|message
        const payload = errMsg.substring(errMsg.indexOf('HOST_KEY_VERIFY|') + 16);
        const parts = payload.split('|');
        const status = parts[0] || 'unknown';
        const host = parts[1] || '';
        const fingerprint = parts[2] || '';
        const keyType = parts[3] || '';
        const message = parts[4] || '';
        hostKeyVerifyState = { status, host, fingerprint, keyType, message, profileID: profile.id, profile };
        return;
      }
      errorMessage = `SSH connection failed: ${errMsg}`;
    }
  }

  async function acceptHostKey() {
    if (!hostKeyVerifyState) return;
    const { profileID, profile } = hostKeyVerifyState;
    try {
      await AcceptSSHHostKey(profileID);
      hostKeyVerifyState = null;
      await connectSSHProfile(profile);
    } catch (e) {
      errorMessage = `Failed to accept host key: ${e.message || e}`;
      hostKeyVerifyState = null;
    }
  }

  function rejectHostKey() {
    if (!hostKeyVerifyState) return;
    RejectSSHHostKey(hostKeyVerifyState.profileID);
    hostKeyVerifyState = null;
  }

  function syncAIToolFlowListsFromConfig() {
    aiToolFlowLists = {
      includeCategories: listToText(aiToolFlowConfig.toolFilter.includeCategories),
      excludeCategories: listToText(aiToolFlowConfig.toolFilter.excludeCategories),
      includeToolIds: listToText(aiToolFlowConfig.toolFilter.includeToolIds),
      excludeToolIds: listToText(aiToolFlowConfig.toolFilter.excludeToolIds),
    };
  }

  function applyAIToolFlowListsToConfig() {
    aiToolFlowConfig = {
      ...aiToolFlowConfig,
      prompt: {
        ...aiToolFlowConfig.prompt,
        maxTerminalContext: Number(aiToolFlowConfig.prompt.maxTerminalContext) || 12000,
      },
      toolFilter: {
        includeCategories: textToList(aiToolFlowLists.includeCategories),
        excludeCategories: textToList(aiToolFlowLists.excludeCategories),
        includeToolIds: textToList(aiToolFlowLists.includeToolIds),
        excludeToolIds: textToList(aiToolFlowLists.excludeToolIds),
      },
    };
  }

  function getEffectiveAIToolPrePrompt() {
    const configured = aiToolFlowConfig?.prompt?.prePrompt?.trim();
    if (configured) {
      return configured;
    }
    return 'You are a terminal automation assistant.';
  }

  function getAIToolPromptPreview() {
    const intro = getEffectiveAIToolPrePrompt();
    const stableIdInstruction = 'Use only the provided tools and refer to them by the exact stable tool id.';
    const selectionShape = '{"toolId":"...","variables":{"Param":"value"},"reason":"..."}';

    return [
      intro,
      '',
      immutableAIToolGuardrails,
      '',
      `Choose the single best registered tool to execute for the user's goal. ${stableIdInstruction} Fill required parameters when possible. If no tool fits, return an empty toolId.`,
      'Return strictly valid JSON with this shape and nothing else:',
      selectionShape,
      '',
      'Terminal type: <active terminal type>',
      'Terminal name: <active terminal name>',
      'Recent terminal output:',
      aiToolFlowConfig.prompt.includeTerminalOutput ? '<trimmed terminal output>' : '<omitted by config>',
      '',
      'User goal:',
      '<goal>',
      '',
      'Available tools:',
      '<tool list derived from current filters>'
    ].join('\n');
  }

  function setDevOpsPrePromptExample() {
    aiToolFlowConfig = {
      ...aiToolFlowConfig,
      prompt: {
        ...aiToolFlowConfig.prompt,
        prePrompt: devopsPrePromptExample,
      },
    };
  }

  function getEditablePromptIntroPreview() {
    const configured = aiToolFlowConfig?.prompt?.prePrompt?.trim();
    if (configured) {
      return configured;
    }
    return devopsPrePromptExample;
  }

  function isUsingDefaultPromptIntroPreview() {
    return !aiToolFlowConfig?.prompt?.prePrompt?.trim();
  }

  $: terminalMap = new Map(terminals.map(t => [t.id, t]));
  $: sidebarGroups = groupedSidebarTerminals(terminals, customFolders);

  function openTemplatePrompt(template, terminalType, terminalId, variableNames) {
    const paramMap = {};
    if (template.parameters) {
      for (const p of template.parameters) {
        paramMap[p.name] = p;
      }
    }
    templatePromptState = {
      templateName: template.name,
      terminalType,
      terminalId,
      fields: variableNames.map((name) => ({
        name,
        label: buildPromptLabel(name),
        value: '',
        discoveryTool: paramMap[name]?.discoveryTool || '',
        suggestions: [],
        loadingSuggestions: false,
      }))
    };
    for (const field of templatePromptState.fields) {
      if (field.discoveryTool) {
        field.loadingSuggestions = true;
        window['go']['main']['App']['RunDiscoveryJSON'](field.discoveryTool, terminalType, '{}')
          .then(raw => {
            const values = JSON.parse(raw);
            field.suggestions = Array.isArray(values) ? values : [];
            field.loadingSuggestions = false;
            templatePromptState = templatePromptState;
          })
          .catch(() => { field.loadingSuggestions = false; templatePromptState = templatePromptState; });
      }
    }
  }

  function templateNameFromTool(tool) {
    return String(tool || '').replace(/^template:/, '');
  }

  function templateForWorkflowStep(step) {
    const templateName = templateNameFromTool(step.tool);
    if (!templateName) return null;
    return templates.find(t => t.name === templateName) || null;
  }

  function workflowStepCommand(template, terminalType) {
    if (!template?.commands) return '';
    return template.commands[terminalType] || (terminalType === 'ssh' ? template.commands.bash : '') || '';
  }

  function buildWorkflowPromptFields(playbook, terminalType) {
    const fields = [];
    for (const [stepIndex, step] of (playbook.steps || []).entries()) {
      if (step.type !== 'run_tool' || !step.tool) continue;
      const template = templateForWorkflowStep(step);
      if (!template) continue;
      const command = workflowStepCommand(template, terminalType);
      const variableNames = command ? extractTemplateVariables(command) : Object.keys(step.inputs || {});
      if (variableNames.length === 0) continue;

      const paramMap = {};
      for (const parameter of (template.parameters || [])) {
        paramMap[parameter.name] = parameter;
      }

      for (const name of variableNames) {
        const currentValue = step.inputs?.[name] || '';
        fields.push({
          name: `${step.id}:${name}`,
          inputName: name,
          stepIndex,
          stepId: step.id,
          label: `${template.name} · ${buildPromptLabel(name)}`,
          value: currentValue,
          discoveryTool: paramMap[name]?.discoveryTool || '',
          suggestions: [],
          loadingSuggestions: false,
          suggestionError: '',
        });
      }
    }
    return fields;
  }

  function workflowPromptVariables(fields) {
    const variables = {};
    for (const field of fields || []) {
      variables[field.inputName || field.name] = field.value || '';
    }
    return variables;
  }

  function loadPromptFieldSuggestions(field, state, terminalType) {
    if (!field.discoveryTool) return;
    field.loadingSuggestions = true;
    field.suggestionError = '';
    window['go']['main']['App']['RunDiscoveryJSON'](
      field.discoveryTool,
      terminalType,
      JSON.stringify(workflowPromptVariables(state.fields))
    )
      .then(raw => {
        const values = JSON.parse(raw);
        field.suggestions = Array.isArray(values) ? values : [];
        field.loadingSuggestions = false;
        templatePromptState = templatePromptState;
      })
      .catch((error) => {
        field.suggestions = [];
        field.loadingSuggestions = false;
        field.suggestionError = error.message || String(error);
        templatePromptState = templatePromptState;
      });
  }

  function openWorkflowPrompt(playbook, activeTerminal) {
    const fields = buildWorkflowPromptFields(playbook, activeTerminal.type);
    if (fields.length === 0) {
      return false;
    }

    templatePromptState = {
      kind: 'workflow',
      templateName: playbook.name,
      workflowPlaybook: playbook,
      terminalType: activeTerminal.type,
      terminalId: activeTerminal.id,
      terminalName: activeTerminal.name || '',
      terminalOutput: activeTerminal.outputBuffer || '',
      fields,
    };

    for (const field of templatePromptState.fields) {
      loadPromptFieldSuggestions(field, templatePromptState, activeTerminal.type);
    }

    return true;
  }

  function closeTemplatePrompt() {
    templatePromptState = null;
  }

  function handleTemplatePromptFieldChange(changedField) {
    if (!templatePromptState || templatePromptState.kind !== 'workflow') {
      return;
    }

    for (const field of templatePromptState.fields || []) {
      if (field === changedField || !field.discoveryTool) continue;
      field.suggestions = [];
      loadPromptFieldSuggestions(field, templatePromptState, templatePromptState.terminalType || '');
    }
  }

  async function submitTemplatePrompt() {
    if (!templatePromptState) {
      return;
    }

    const variables = {};
    for (const field of templatePromptState.fields) {
      if (!field.value.trim()) {
        errorMessage = `Please enter a value for ${field.label}.`;
        return;
      }
      variables[field.name] = field.value.trim();
    }

    try {
      if (templatePromptState.kind === 'workflow') {
        await runPromptedWorkflow(templatePromptState, variables);
        errorMessage = "";
        closeTemplatePrompt();
        return;
      }

      await window['go']['main']['App']['ApplyTemplateWithVariables'](
        templatePromptState.terminalId,
        templatePromptState.templateName,
        templatePromptState.terminalType,
        variables
      );
      errorMessage = "";
      closeTemplatePrompt();
    } catch (error) {
      errorMessage = `Failed to apply template: ${error.message || error}`;
    }
  }

  async function runPromptedWorkflow(promptState, variables) {
    const playbook = promptState.workflowPlaybook;
    const steps = (playbook.steps || []).map((step) => ({
      ...step,
      inputs: step.inputs ? { ...step.inputs } : {},
    }));

    for (const field of promptState.fields || []) {
      const stepIndex = Number(field.stepIndex);
      if (!steps[stepIndex]) continue;
      steps[stepIndex].inputs = {
        ...(steps[stepIndex].inputs || {}),
        [field.inputName || field.name]: variables[field.name],
      };
    }

    const definition = JSON.stringify({
      id: playbook.id,
      name: playbook.name,
      description: playbook.description || '',
      mode: playbook.mode || 'assist',
      steps,
    });

    await window['go']['main']['App']['RunWorkflowDraftJSON'](
      definition,
      Number(promptState.terminalId),
      promptState.terminalType || '',
      promptState.terminalName || '',
      promptState.terminalOutput || ''
    );
  }

  // ─── Layout tree helpers ───────────────────────────────

	  function finalizeTerminalRemoval(id) {
	    const existing = terminals.find(t => t.id === id);
	    cleanupTerminalResources(existing, { dispose: false });
	    const nextTerminals = terminals.filter(t => t.id !== id);
	    terminals = nextTerminals;
	    layoutTree = removeLeafFromTree(layoutTree, id);

    if (activeTerminalId === id) {
      const visibleIds = layoutTree ? collectLeafIds(layoutTree) : [];
      const nextActive =
        nextTerminals.find((terminal) => visibleIds.includes(terminal.id)) ||
        nextTerminals[0] ||
        null;
      activeTerminalId = nextActive ? nextActive.id : null;
    }

	    RemoveTerminalState(id);
	    scheduleSessionSave();
	  }

	  function cleanupTerminalResources(term, { dispose = true } = {}) {
	    if (!term) return;

	    for (const cleanup of term.cleanupHandlers || []) {
	      try {
	        if (typeof cleanup === 'function') cleanup();
	        else if (cleanup && typeof cleanup.dispose === 'function') cleanup.dispose();
	      } catch (error) {
	        console.error(`Failed to clean up terminal ${term.id}:`, error);
	      }
	    }
	    term.cleanupHandlers = [];

	    if (dispose) {
	      safelyDisposeTerminal(term);
	    }
	  }

  // ─── Terminal instance creation ────────────────────────

  async function createTerminalInstance(id, type, name, minimized, sshProfileId = '', restoring = false, existingTmuxSessionName = '', existingResumeId = '', initialRestoreClass = 'fresh') {
    const terminal = new Terminal({
      cursorBlink: true,
      cursorStyle: 'bar',
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
      fontSize: 13,
      lineHeight: 1.35,
      scrollback: 100000,
      theme: {
        background: '#0c0e14',
        foreground: '#c9d1d9',
        cursor: '#63b3ed',
        cursorAccent: '#0c0e14',
        selectionBackground: 'rgba(99, 179, 237, 0.25)',
        selectionForeground: '#ffffff',
        black: '#1a1e2e',
        red: '#f47067',
        green: '#7ee787',
        yellow: '#e3b341',
        blue: '#63b3ed',
        magenta: '#d2a8ff',
        cyan: '#76e4f7',
        white: '#c9d1d9',
        brightBlack: '#545d68',
        brightRed: '#ff7b72',
        brightGreen: '#7ee787',
        brightYellow: '#f0c74f',
        brightBlue: '#79c0ff',
        brightMagenta: '#d6b4fc',
        brightCyan: '#9aedfe',
        brightWhite: '#f0f3f6'
      }
    });
    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    const searchAddon = new SearchAddon();
    terminal.loadAddon(searchAddon);

    const newTerminal = {
      id,
      terminal,
      fitAddon,
      searchAddon,
      minimized,
      name,
      editingName: false,
      type,
      outputBuffer: '',
      sshProfileId,
      disconnected: false,
      reconnecting: false,
      searchVisible: false,
      searchQuery: '',
      tmuxSessionName: '',
      tmuxOwned: false,
      tmuxActive: false,
      tmuxMode: '',
      tmuxStatus: '',
      tmuxError: '',
      rcMode: '',
      rcStatus: '',
      shellPath: '',
      resumeId: existingResumeId || generateResumeId(),
	      restoreClass: initialRestoreClass,
	      restoredTranscript: '',
	      restoreDismissed: false,
	      recording: false,
	      folderId: '',
	      cleanupHandlers: []
	    };
    terminals = [...terminals, newTerminal];

    await tick();

    // DOM setup — only when terminal is visible (not minimized)
    const element = document.getElementById(`terminal-${id}`);
    if (element) {
      if (safelyAttachTerminal(newTerminal, element)) {
        try {
          terminal.focus();
        } catch (error) {
          console.error(`Failed to focus terminal ${id}:`, error);
        }
        safelyWriteTerminal(newTerminal, '\x1b[2J\x1b[H');
        safelyFitAndResizeTerminal(newTerminal, ResizeTerminal);
      }

	      const inputDisposable = terminal.onData(data => {
	        WriteToTerminal(id, data);
	      });
	      newTerminal.cleanupHandlers.push(inputDisposable);

	      const handlePaste = (event) => {
	        const pasteData = event.clipboardData.getData('text');
	        if (pasteData) {
	          WriteToTerminal(id, pasteData);
	        }
	        event.preventDefault();
	      };
	      element.addEventListener('paste', handlePaste);
	      newTerminal.cleanupHandlers.push(() => element.removeEventListener('paste', handlePaste));
	    } else if (!minimized) {
      errorMessage = `Failed to find terminal element for ID: ${id}`;
      console.error(errorMessage);
      return newTerminal;
    }

	    // Event listeners + backend init — always run (even for minimized)
	    const offOutput = EventsOn(`terminal-output-${id}`, data => {
	      safelyWriteTerminal(newTerminal, data);
	      terminals = terminals.map(t => {
	        if (t.id !== id) return t;
        const nextOutput = (t.outputBuffer + data).slice(-12000);
        return { ...t, outputBuffer: nextOutput };
      });
	      window['go']['main']['App']['AppendTerminalTranscript'](newTerminal.resumeId, data).catch(() => {});
	    });
	    newTerminal.cleanupHandlers.push(offOutput);

	    const offClosed = EventsOn(`terminal-closed-${id}`, () => {
	      const term = terminals.find(t => t.id === id);
	      if (term) {
	        cleanupTerminalResources(term);
	      }
	      finalizeTerminalRemoval(id);
	      reinitializeTerminals();
	    });
	    newTerminal.cleanupHandlers.push(offClosed);

	    const offDisconnected = EventsOn(`terminal-disconnected-${id}`, () => {
	      terminals = terminals.map(t => {
	        if (t.id !== id) return t;
	        return { ...t, disconnected: true, reconnecting: false };
	      });
	    });
	    newTerminal.cleanupHandlers.push(offDisconnected);

    await ConfirmFrontendReady(id);
    await InitializeTerminal(id);

    try {
      const status = await readTmuxStatus(id);
      terminals = terminals.map((t) => {
        if (t.id !== id) return t;
        return {
          ...t,
          tmuxActive: Boolean(status?.active),
          tmuxSessionName: status?.sessionName || existingTmuxSessionName || '',
          tmuxMode: status?.mode || '',
          tmuxStatus: status?.status || '',
          tmuxError: status?.error || '',
          rcMode: status?.rcMode || '',
          rcStatus: status?.rcStatus || '',
          shellPath: status?.shellPath || '',
          tmuxOwned: Boolean(status?.active) && type !== 'ssh' && !restoring
        };
      });
      newTerminal.tmuxActive = Boolean(status?.active);
      newTerminal.tmuxSessionName = status?.sessionName || existingTmuxSessionName || '';
      newTerminal.tmuxMode = status?.mode || '';
      newTerminal.tmuxStatus = status?.status || '';
      newTerminal.tmuxError = status?.error || '';
      newTerminal.rcMode = status?.rcMode || '';
      newTerminal.rcStatus = status?.rcStatus || '';
      newTerminal.shellPath = status?.shellPath || '';
      newTerminal.tmuxOwned = Boolean(status?.active) && type !== 'ssh' && !restoring;
    } catch (error) {
      console.error(`Failed to read tmux status for terminal ${id}:`, error);
    }

    if (!tmuxCapableTerminalTypes.has(type) && type !== 'ssh') {
      // cmd/powershell: no tmux
      await WriteToTerminal(id, '\r');
      let promptCmd = '';
      switch (type) {
        case 'powershell':
          promptCmd = 'function prompt { "$env:USERNAME ❯ " }; cls';
          break;
        case 'cmd':
          promptCmd = 'prompt %USERNAME% $G$S& cls';
          break;
      }
      if (promptCmd) {
        await WriteToTerminal(id, promptCmd + '\r');
      }
    }

    return newTerminal;
  }

  // ─── Add / Split / Remove terminals ────────────────────

  async function addTerminal(terminalTypeParam, nameParam, minimized = false, initialPath = '') {
    const type = typeof terminalTypeParam === 'string' ? terminalTypeParam : selectedTerminalType;

    // SSH terminals go through the profile picker instead
    if (type === 'ssh') {
      openSSHProfilePicker();
      return;
    }

    const name = typeof nameParam === 'string' ? nameParam : `${type.toUpperCase()} ${terminals.length + 1}`;

    try {
      const tmuxSessionName = tmuxCapableTerminalTypes.has(type) ? generateTmuxSessionName('mimir') : '';
      const id = await startTerminalBackend(type, tmuxSessionName);
      if (!id) {
        errorMessage = "Failed to start terminal. The backend returned an invalid ID.";
        return;
      }

      // Insert into layout tree BEFORE createTerminalInstance so DOM element exists
      const newLeaf = { type: 'leaf', terminalId: id };
      if (!minimized) {
        if (layoutTree === null) {
          layoutTree = newLeaf;
        } else {
          layoutTree = {
            type: 'split',
            direction: 'horizontal',
            ratio: 0.5,
            children: [layoutTree, newLeaf]
          };
        }
      }

      const newTerminal = await createTerminalInstance(id, type, name, minimized, '', false, tmuxSessionName, '', 'fresh');
      activeTerminalId = id;
      persistTerminalState(newTerminal);

      // Re-attach all existing terminals whose DOM elements were recreated by the tree change
      await reinitializeTerminals();

      if (initialPath) {
        let cdCommand = '';
        switch(type) {
          case 'cmd':
            cdCommand = `cd /d "${initialPath}"`;
            break;
          case 'powershell':
            cdCommand = `Set-Location -LiteralPath "${initialPath}"`;
            break;
          case 'wsl':
          case 'bash':
          case 'zsh':
            cdCommand = `cd ${shellQuotePath(initialPath)}`;
            break;
        }
        if (cdCommand) {
          await WriteToTerminal(id, cdCommand + '\r');
        }
      }
	    } catch (error) {
	      errorMessage = formatErrorMessage(error);
	      console.error(`addTerminal: ${errorMessage}`, error);
	    }
	  }

  async function splitTerminal(terminalId, direction) {
    const sourceTerm = terminals.find(t => t.id === terminalId);
    if (!sourceTerm) return;

    const type = sourceTerm.type;

    try {
      let newId;
      let name;
      let sshProfileId = '';

      if (type === 'ssh' && sourceTerm.sshProfileId) {
        const profile = sshProfiles.find(p => p.id === sourceTerm.sshProfileId);
        if (!profile) {
          errorMessage = 'SSH profile not found. Cannot split this terminal.';
          return;
        }
        newId = await StartSSHTerminal(profile.id);
        name = `SSH: ${profile.name}`;
        sshProfileId = profile.id;
      } else {
        const tmuxSessionName = tmuxCapableTerminalTypes.has(type) ? generateTmuxSessionName('mimir') : '';
        newId = await startTerminalBackend(type, tmuxSessionName);
        name = `${type.toUpperCase()} ${terminals.length + 1}`;
      }
      if (!newId) return;

      const newLeaf = { type: 'leaf', terminalId: newId };
      layoutTree = replaceLeaf(layoutTree, terminalId, {
        type: 'split',
        direction: direction,
        ratio: 0.5,
        children: [
          { type: 'leaf', terminalId: terminalId },
          newLeaf
        ]
      });

      const newTerminal = await createTerminalInstance(newId, type, name, false, sshProfileId, false, '', '', 'fresh');
      persistTerminalState({ ...newTerminal, minimized: false, sshProfileId });
      activeTerminalId = newId;

      await reinitializeTerminals();
    } catch (error) {
      errorMessage = error.message || 'Failed to split terminal.';
    }
  }

  async function toggleRecording(terminalId) {
    const term = terminals.find(t => t.id === terminalId);
    if (!term) return;

    try {
      if (term.recording) {
        await StopRecording(terminalId);
        term.recording = false;
      } else {
        const name = term.name || `Terminal ${terminalId}`;
        await StartRecording(terminalId, name);
        term.recording = true;
      }
      terminals = terminals;
    } catch (e) {
      console.error('Recording toggle failed:', e);
    }
  }

  function removeTerminal(id) {
    const term = terminals.find(t => t.id === id);
    // Kill the tmux session when user explicitly closes the terminal
    if (term?.tmuxSessionName && term?.tmuxOwned) {
      KillTmuxSession(term.tmuxSessionName).catch(() => {});
    }
    if (term && term.type === 'ssh' && term.disconnected) {
      // Disconnected SSH: clean up backend + remove from UI immediately
      CloseSSHTerminalFull(id);
      safelyDisposeTerminal(term, 'disconnected terminal');
      finalizeTerminalRemoval(id);
      reinitializeTerminals();
      return;
    }
    // Close backend session — this triggers the read-loop goroutine to exit,
    // which emits terminal-closed-{id}. If the event listener was never
    // registered (race condition), clean up the UI immediately as fallback.
    CloseTerminal(id);
    // Fallback cleanup: if terminal-closed event handler was not registered
    // (e.g. DOM element not found during createTerminalInstance), do it now.
    setTimeout(() => {
      const stillExists = terminals.find(t => t.id === id);
      if (stillExists) {
        safelyDisposeTerminal(stillExists, 'fallback terminal');
        finalizeTerminalRemoval(id);
        reinitializeTerminals();
      }
    }, 500);
  }

  async function reconnectSSH(id) {
    const term = terminals.find(t => t.id === id);
    terminals = terminals.map(t => {
      if (t.id !== id) return t;
      return { ...t, reconnecting: true };
    });
    try {
      await ReconnectSSHTerminal(id);
      await ConfirmFrontendReady(id);
      await InitializeTerminal(id);
      terminals = terminals.map(t => {
        if (t.id !== id) return t;
        safelyWriteTerminal(t, '\r\n\x1b[32m--- Reconnected ---\x1b[0m\r\n');
        return { ...t, disconnected: false, reconnecting: false };
      });
    } catch (e) {
      terminals = terminals.map(t => {
        if (t.id !== id) return t;
        safelyWriteTerminal(t, `\r\n\x1b[31mReconnect failed: ${e.message || e}\x1b[0m\r\n`);
        return { ...t, reconnecting: false };
      });
    }
  }

  // ─── Terminal Search ──────────────────────────────────

  function toggleTerminalSearch() {
    if (!activeTerminalId) return;
    terminals = terminals.map(t => {
      if (t.id !== activeTerminalId) return t;
      return { ...t, searchVisible: !t.searchVisible, searchQuery: t.searchVisible ? '' : t.searchQuery };
    });
    const term = terminals.find(t => t.id === activeTerminalId);
    if (term && !term.searchVisible) {
      term.searchAddon.clearDecorations();
      term.terminal.focus();
    }
  }

  function closeTerminalSearch(id) {
    terminals = terminals.map(t => {
      if (t.id !== id) return t;
      return { ...t, searchVisible: false, searchQuery: '' };
    });
    const term = terminals.find(t => t.id === id);
    if (term) {
      term.searchAddon.clearDecorations();
      term.terminal.focus();
    }
  }

  function terminalSearchNext(id) {
    const term = terminals.find(t => t.id === id);
    if (term && term.searchQuery) {
      term.searchAddon.findNext(term.searchQuery);
    }
  }

  function terminalSearchPrev(id) {
    const term = terminals.find(t => t.id === id);
    if (term && term.searchQuery) {
      term.searchAddon.findPrevious(term.searchQuery);
    }
  }

  function updateTerminalSearchQuery(id, query) {
    terminals = terminals.map(t => {
      if (t.id !== id) return t;
      return { ...t, searchQuery: query };
    });
    const term = terminals.find(t => t.id === id);
    if (term) {
      if (query) {
        term.searchAddon.findNext(query);
      } else {
        term.searchAddon.clearDecorations();
      }
    }
  }

  function dismissRestoreSummary(id) {
    terminals = terminals.map((t) => {
      if (t.id !== id) return t;
      return { ...t, restoreDismissed: true };
    });
  }

  function openTranscriptViewer(id) {
    const term = terminals.find((t) => t.id === id);
    transcriptViewerState = {
      resumeId: term?.resumeId || '',
      label: term?.name || '',
    };
  }

  function closeTranscriptViewer() {
    transcriptViewerState = null;
  }

  function handleGlobalKeydown(event) {
    // Ctrl+Shift+N → toggle notes panel
    if (event.ctrlKey && event.shiftKey && event.key === 'N') {
      event.preventDefault();
      notesPanelOpen = !notesPanelOpen;
      setTimeout(handleResize, 50);
      return;
    }
    // Ctrl+Shift+F → toggle terminal search
    if (event.ctrlKey && event.shiftKey && event.key === 'F') {
      event.preventDefault();
      toggleTerminalSearch();
      return;
    }
    // Ctrl+Shift+P → toggle template picker (search & apply a template)
    if (event.ctrlKey && event.shiftKey && (event.key === 'P' || event.key === 'p')) {
      event.preventDefault();
      showTemplatePicker = !showTemplatePicker;
      return;
    }
    // Ctrl+Shift+W → toggle workflow picker (search & run a workflow)
    if (event.ctrlKey && event.shiftKey && (event.key === 'W' || event.key === 'w')) {
      event.preventDefault();
      toggleWorkflowPicker();
      return;
    }
    // Escape → close pickers, then any search bars
    if (event.key === 'Escape') {
      if (showTemplatePicker) {
        showTemplatePicker = false;
        return;
      }
      if (showWorkflowPicker) {
        showWorkflowPicker = false;
        return;
      }
      const anySearchVisible = terminals.some(t => t.searchVisible);
      if (anySearchVisible) {
        terminals.forEach(t => {
          if (t.searchVisible) closeTerminalSearch(t.id);
        });
      }
    }
  }

  // ─── Minimize / Restore ────────────────────────────────

  async function terminalToBackground(id) {
    terminals = terminals.map(t => {
      if (t.id === id) {
        const next = { ...t, minimized: true };
        persistTerminalState(next);
        return next;
      }
      return t;
    });
    layoutTree = removeLeafFromTree(layoutTree, id);
    // Re-attach remaining terminals whose DOM was recreated by tree collapse
    await reinitializeTerminals();
  }

  async function terminalToForeground(id) {
    terminals = terminals.map(t => {
      if (t.id === id) {
        const next = { ...t, minimized: false };
        persistTerminalState(next);
        return next;
      }
      return t;
    });

    // Re-insert into layout tree
    const newLeaf = { type: 'leaf', terminalId: id };
    if (layoutTree === null) {
      layoutTree = newLeaf;
    } else {
      layoutTree = {
        type: 'split',
        direction: 'horizontal',
        ratio: 0.5,
        children: [layoutTree, newLeaf]
      };
    }

    // Re-attach ALL terminals (including restored one) to their new DOM elements
    await reinitializeTerminals();
  }

  async function toggleMinimize(id) {
    const term = terminals.find(t => t.id === id);
    if (term) {
      if (term.minimized) {
        await terminalToForeground(id);
      } else {
        await terminalToBackground(id);
      }
    }
  }

  // ─── Name editing ──────────────────────────────────────

  function startEditingName(id) {
    terminals = terminals.map(t => {
      if (t.id === id) return { ...t, editingName: true };
      return t;
    });
    tick().then(() => {
      const el = document.getElementById(`terminal-name-input-${id}`);
      if (el) el.focus();
    });
  }

  function saveTerminalName(id, event) {
    terminals = terminals.map(t => {
      if (t.id === id) {
        const newName = event.target.value;
        const next = { ...t, name: newName, editingName: false };
        persistTerminalState(next);
        return next;
      }
      return t;
    });
  }

  // ─── Templates ─────────────────────────────────────────

  async function applyTemplate(templateName) {
    if (activeTerminalId === null) {
      errorMessage = "Please select a terminal first.";
      return;
    }
    const activeTerminal = terminals.find(t => t.id === activeTerminalId);
    if (!activeTerminal) {
      errorMessage = "Active terminal not found.";
      return;
    }

    const template = templates.find(t => t.name === templateName);
    if (!template) {
      errorMessage = `Template '${templateName}' not found.`;
      return;
    }
    if (!template.commands) {
      errorMessage = `Template '${templateName}' has no 'commands' property.`;
      return;
    }

    const termType = activeTerminal.type;
    const commandToExecute = template.commands[termType] || (termType === 'ssh' ? template.commands['bash'] : null);
    if (!commandToExecute) {
      errorMessage = `Template '${templateName}' does not have a command for terminal type '${termType}'.`;
      return;
    }

    const requiredVariables = extractTemplateVariables(commandToExecute);
    if (requiredVariables.length > 0) {
      openTemplatePrompt(template, activeTerminal.type, activeTerminal.id, requiredVariables);
      return;
    }

    try {
      await ApplyTemplate(activeTerminal.id, templateName, activeTerminal.type);
      errorMessage = "";
    } catch (error) {
      errorMessage = `Failed to apply template: ${error.message || error}`;
    }
  }

  async function toggleWorkflowPicker() {
    if (showWorkflowPicker) {
      showWorkflowPicker = false;
      return;
    }
    if (activeTerminalId === null) {
      errorMessage = $tr('workflowPicker.noTerminal');
      return;
    }
    showWorkflowPicker = true;
    workflowPickerLoading = true;
    try {
      const payload = await window['go']['main']['App']['GetPlaybooksJSON']();
      workflowPickerPlaybooks = JSON.parse(payload);
    } catch (error) {
      workflowPickerPlaybooks = [];
      errorMessage = `Failed to load workflows: ${error.message || error}`;
    } finally {
      workflowPickerLoading = false;
    }
  }

  async function runWorkflowFromPicker(playbook) {
    const activeTerminal = terminals.find(t => t.id === activeTerminalId);
    if (!activeTerminal) {
      errorMessage = 'Active terminal not found.';
      return;
    }
    if (openWorkflowPrompt(playbook, activeTerminal)) {
      errorMessage = '';
      return;
    }
    try {
      const definition = JSON.stringify({
        id: playbook.id,
        name: playbook.name,
        description: playbook.description || '',
        mode: playbook.mode || 'assist',
        steps: playbook.steps || [],
      });
      await window['go']['main']['App']['RunWorkflowDraftJSON'](
        definition,
        Number(activeTerminal.id),
        activeTerminal.type || '',
        activeTerminal.name || '',
        activeTerminal.outputBuffer || ''
      );
      errorMessage = '';
    } catch (error) {
      errorMessage = `Failed to run workflow: ${error.message || error}`;
    }
  }

  function openAIPanel(mode) {
    if (activeTerminalId === null) {
      errorMessage = 'Please select a terminal first.';
      return;
    }

    const activeTerminal = terminals.find(t => t.id === activeTerminalId);
    if (!activeTerminal) {
      errorMessage = 'Active terminal not found.';
      return;
    }

    aiPanelState = {
      mode,
      terminalId: activeTerminal.id,
      terminalType: activeTerminal.type,
      terminalName: activeTerminal.name,
      goal: '',
      result: '',
      loading: false,
    };
    showAIMenu = false;
  }

  function closeAIPanel() {
    aiPanelState = null;
  }

  async function openFunctionCatalog() {
    try {
      const payload = await window['go']['main']['App']['GetFunctionCatalogJSON']();
      functionCatalog = JSON.parse(payload);
      showFunctionCatalog = true;
      showAIMenu = false;
      errorMessage = '';
    } catch (error) {
      errorMessage = `Failed to load function catalog: ${error.message || error}`;
    }
  }

  function getDiscoveryTerminalType() {
    const activeTerminal = terminals.find((terminal) => terminal.id === activeTerminalId);
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
    showFunctionCatalog = false;
    openPage("workflowBuilder");
  }

  async function loadAIProviders() {
    try {
      const raw = await window['go']['main']['App']['GetAIProvidersJSON']();
      aiProviders = JSON.parse(raw);
    } catch (error) {
      console.error('Failed to load AI providers:', error);
      aiProviders = [];
    }
  }

  async function openAISettings() {
    await loadAIProviders();
    syncAIToolFlowListsFromConfig();
    showAISettings = true;
    showAIMenu = false;
  }

  function closeAISettings() {
    showAISettings = false;
  }

  function toggleAIMenu() {
    showAIMenu = !showAIMenu;
  }

  function applyAISettingsDefaults(provider) {
    const desc = aiProviders.find((p) => p.id === provider);
    if (!desc) return;
    // Adopt the new provider's defaults when the current value is empty or was
    // simply another provider's default. Custom values are preserved. The API
    // key is no longer wiped — it is optional for every provider.
    const otherModelDefaults = aiProviders.filter((p) => p.id !== provider).map((p) => p.defaultModel).filter(Boolean);
    if (!aiSettings.model || otherModelDefaults.includes(aiSettings.model)) {
      aiSettings.model = desc.defaultModel;
    }
    const otherUrlDefaults = aiProviders.filter((p) => p.id !== provider).map((p) => p.defaultBaseUrl).filter(Boolean);
    if (!aiSettings.baseUrl || otherUrlDefaults.includes(aiSettings.baseUrl)) {
      aiSettings.baseUrl = desc.defaultBaseUrl;
    }
    aiSettings = { ...aiSettings };
  }

  async function saveAISettings() {
    try {
      applyAIToolFlowListsToConfig();
      const saved = JSON.parse(await window['go']['main']['App']['UpdateAISettingsJSON'](JSON.stringify(aiSettings)));
      aiSettings = { ...saved };
      const savedFlowConfig = JSON.parse(await window['go']['main']['App']['UpdateAIToolFlowConfigJSON'](JSON.stringify(aiToolFlowConfig)));
      aiToolFlowConfig = normalizeAIToolFlowConfig(savedFlowConfig);
      syncAIToolFlowListsFromConfig();
      showAISettings = false;
      errorMessage = '';
    } catch (error) {
      errorMessage = `Failed to save AI settings: ${error.message || error}`;
    }
  }

  async function insertFileIntoActiveTerminal(event) {
    if (activeTerminalId === null) {
      errorMessage = 'Please select a terminal first.';
      return;
    }

    try {
      await WriteToTerminal(activeTerminalId, event.detail.content);
      errorMessage = '';
    } catch (error) {
      errorMessage = `Failed to insert file into terminal: ${error.message || error}`;
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
      notesPanelOpen = true;
      if (currentPage !== 'terminals') {
        await openPage('terminals');
      }
      setTimeout(handleResize, 50);
    } catch (err) {
      errorMessage = `Failed to import note: ${err}`;
    }
  }

  function editTemplate(template) {
    templateToEdit = template;
    currentPage = "templateManager";
  }

  // ─── Resize ────────────────────────────────────────────

  function handleResize() {
    requestAnimationFrame(() => {
      terminals.forEach(t => {
        if (!t.minimized) {
          safelyFitAndResizeTerminal(t, ResizeTerminal);
        }
      });
    });
  }

  function startNotesDrag(e) {
    e.preventDefault();
    const startX = e.clientX;
    const startWidth = notesPanelWidth;
    const onMove = (ev) => {
      notesPanelWidth = Math.max(250, Math.min(800, startWidth + (startX - ev.clientX)));
    };
    const onUp = () => {
      localStorage.setItem('mimir-notes-width', String(notesPanelWidth));
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
      handleResize();
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  }

  async function reinitializeTerminals() {
    await tick();
    for (const t of terminals) {
      if (!t.minimized) {
        const element = document.getElementById(`terminal-${t.id}`);
        if (element) {
          if (safelyAttachTerminal(t, element)) {
            safelyFitAndResizeTerminal(t, ResizeTerminal);
          }
        }
      }
    }
  }

  // ─── Drag & Drop (swaps terminals in tree) ─────────────

  function handleDragStart(event, id) {
    draggedTerminalId = id;
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', id);
    setTimeout(() => {
      event.currentTarget.classList.add('dragging');
    }, 0);
  }

  function handleDragOver(event, id) {
    event.preventDefault();
    const targetWrapper = event.currentTarget;
    if (targetWrapper.classList.contains('terminal-header') && draggedTerminalId !== id) {
      const bounding = targetWrapper.getBoundingClientRect();
      const offset = event.clientY - bounding.top;
      if (offset > bounding.height / 2) {
        targetWrapper.classList.remove('drag-over-top');
        targetWrapper.classList.add('drag-over-bottom');
      } else {
        targetWrapper.classList.remove('drag-over-bottom');
        targetWrapper.classList.add('drag-over-top');
      }
    }
  }

  function handleDragLeave(event) {
    event.currentTarget.classList.remove('drag-over-top', 'drag-over-bottom');
  }

  function handleDrop(event, targetId) {
    event.preventDefault();
    event.currentTarget.classList.remove('drag-over-top', 'drag-over-bottom');

    const draggedId = parseInt(event.dataTransfer.getData('text/plain'));
    if (draggedId && draggedId !== targetId) {
      layoutTree = swapLeaves(layoutTree, draggedId, targetId);
      reinitializeTerminals();
    }
  }

  function handleDragEnd(event) {
    event.target.classList.remove('dragging');
    draggedTerminalId = null;
    document.querySelectorAll('.drag-over-top, .drag-over-bottom').forEach(el => {
      el.classList.remove('drag-over-top', 'drag-over-bottom');
    });
  }

  // ─── Reactive statements ───────────────────────────────

  $: visibleTerminalCount = layoutTree ? collectLeafIds(layoutTree).length : 0;

  $: filteredTemplates = normalizeTemplates(templates).filter(template =>
    template.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    template.description.toLowerCase().includes(searchQuery.toLowerCase())
  );

  async function loadTemplates() {
    templates = normalizeTemplates(await GetTemplates());
    errorMessage = "";
  }

  async function openPage(page) {
    currentPage = page;

    if (page === "terminals") {
      await reinitializeTerminals();
      return;
    }

    if (page === "recordings" || page === "settings") {
      aggAvailable = await IsAggInstalled().catch(() => false);
      aggStatus = await window['go']['main']['App']['GetAggStatus']().catch(() => 'missing');
    }

    if (page === "recordings") {
      try {
        recordingList = await ListRecordings();
      } catch (e) {
        console.error('Failed to load recordings:', e);
      }
    }

    if (page === "templateManager") {
      try {
        await loadTemplates();
      } catch (error) {
        errorMessage = `Failed to load templates: ${error.message || error}`;
      }
    }

    if (page === "fileBrowser") {
      // Auto-detect if active terminal is SSH
      const activeTerm = terminals.find(t => t.id === activeTerminalId);
      if (activeTerm && activeTerm.type === 'ssh' && !activeTerm.disconnected) {
        try {
          const label = await GetSSHTerminalLabel(activeTerm.id);
          if (label) {
            fileBrowserRemoteTerminalId = activeTerm.id;
            fileBrowserRemoteLabel = label;
            return;
          }
        } catch (e) {
          // Fall through to local mode
        }
      }
      fileBrowserRemoteTerminalId = 0;
      fileBrowserRemoteLabel = '';
    }
  }

  async function restoreLocalTerminal(saved) {
    const tmuxSessionName = tmuxCapableTerminalTypes.has(saved.type)
      ? (saved.tmuxSessionName || generateTmuxSessionName('mimir'))
      : '';
    const id = await startTerminalBackend(saved.type, tmuxSessionName);
    if (!id) return;
    const newLeaf = { type: 'leaf', terminalId: id };
    if (saved.minimized) {
      // minimized: don't add to layout tree
    } else if (layoutTree === null) {
      layoutTree = newLeaf;
    } else {
      layoutTree = { type: 'split', direction: 'horizontal', ratio: 0.5, children: [layoutTree, newLeaf] };
    }
    const restoreClass = ['bash', 'zsh', 'wsl'].includes(saved.type) ? 'rehydrated' : 'transcript-restored';
    const newTerminal = await createTerminalInstance(
      id,
      saved.type,
      saved.name,
      saved.minimized,
      '',
      true,
      tmuxSessionName,
      saved.resumeId || '',
      restoreClass,
    );
    const restoredTranscript = await loadTranscriptExcerpt(newTerminal.resumeId);
    const folderId = saved.folderId || '';
    terminals = terminals.map((t) => t.id === newTerminal.id ? { ...t, restoredTranscript, restoreClass, restoreDismissed: false, folderId } : t);
    persistTerminalState({ ...newTerminal, restoreClass, folderId });
    await reinitializeTerminals();
  }

  async function restoreSSHTerminal(profile, saved) {
    try {
      const id = await StartSSHTerminal(profile.id);
      if (!id) return;
      const newLeaf = { type: 'leaf', terminalId: id };
      if (saved.minimized) {
        // minimized: don't add to layout tree
      } else if (layoutTree === null) {
        layoutTree = newLeaf;
      } else {
        layoutTree = { type: 'split', direction: 'horizontal', ratio: 0.5, children: [layoutTree, newLeaf] };
      }
      const name = saved.name || `SSH: ${profile.name}`;
      const restoreClass = 'live-restored';
      const newTerminal = await createTerminalInstance(
        id,
        'ssh',
        name,
        saved.minimized,
        profile.id,
        true,
        saved.tmuxSessionName || '',
        saved.resumeId || '',
        restoreClass,
      );
      const restoredTranscript = await loadTranscriptExcerpt(newTerminal.resumeId);
      const folderId = saved.folderId || '';
      terminals = terminals.map((t) => t.id === newTerminal.id ? { ...t, restoredTranscript, restoreClass, restoreDismissed: false, folderId } : t);
      persistTerminalState({ ...newTerminal, type: 'ssh', sshProfileId: profile.id, restoreClass, folderId });
      await reinitializeTerminals();
    } catch (e) {
      // Server offline — silently skip this terminal
      console.warn(`Failed to restore SSH terminal for ${profile.name}: ${e.message || e}`);
    }
  }

	  onMount(async () => {
	    window.addEventListener('resize', handleResize);
      await loadSSHProfiles();
	    try {
	      await loadAvailableTerminalTypes();
	      await loadTemplates();
      await loadCustomFolders();
      historyTrackingEnabled = await IsHistoryTrackingEnabled();
      aiSettings = JSON.parse(await window['go']['main']['App']['GetAISettingsJSON']());
      aiToolFlowConfig = normalizeAIToolFlowConfig(JSON.parse(await window['go']['main']['App']['GetAIToolFlowConfigJSON']()));
      syncAIToolFlowListsFromConfig();

      try {
        const pendingRaw = await window['go']['main']['App']['GetPendingUpdate']();
        const pending = pendingRaw ? JSON.parse(pendingRaw) : null;
        if (pending?.version) updateInstalled = true;
      } catch (_) {}

      const savedSession = await GetLoadedSessionData();
      const savedTerminals = dedupeSavedSessionTerminals(savedSession?.terminals || []);
      if (savedTerminals.length > 0) {
        for (const saved of savedTerminals) {
          if (saved.type === 'ssh' && saved.sshProfileId) {
            const profile = sshProfiles.find(p => p.id === saved.sshProfileId);
            if (profile) await restoreSSHTerminal(profile, saved);
          } else if (['bash', 'zsh', 'wsl', 'cmd', 'powershell'].includes(saved.type)) {
            await restoreLocalTerminal(saved);
          }
        }
      }
      // Fallback: if no terminals were restored, create a default one
      if (terminals.length === 0) addTerminal();
      else scheduleSessionSave(50);
    } catch (error) {
      errorMessage = `Failed to load templates or session: ${error.message || error}`;
    }
  });

    const offUpdateProgress = EventsOn('update-progress', (data) => {
      try {
        const progress = JSON.parse(data);
        updateProgress = progress;
        if (progress.stage === 'done') {
          updateDownloading = false;
          updateInstalled = true;
        } else if (progress.stage === 'error') {
          updateDownloading = false;
          errorMessage = `Update failed: ${progress.error}`;
        }
      } catch (_) {}
    });

	  onDestroy(() => {
	    window.removeEventListener('resize', handleResize);
      offUpdateProgress();
	    terminals.forEach((term) => cleanupTerminalResources(term));
	    if (sessionSaveTimer) {
	      clearTimeout(sessionSaveTimer);
	      sessionSaveTimer = null;
    }
  });
</script>

<svelte:window on:keydown={handleGlobalKeydown} />

<SecretUnlockGate />

<main>
  <Sidebar
    currentPage={currentPage}
    terminals={terminals}
    sshProfiles={sshProfiles}
    customFolders={customFolders}
    groups={sidebarGroups}
    foldersOpen={terminalSessionFoldersOpen}
    activeTerminalId={activeTerminalId}
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
  />

  <AppMainContent
    bind:currentPage
    {availableTerminalTypes}
    bind:selectedTerminalType
    bind:aiSettings
    {visibleTerminalCount}
    {errorMessage}
    bind:layoutTree
    {terminalMap}
    bind:activeTerminalId
    bind:notesPanelOpen
    {notesPanelWidth}
    bind:terminals
    {showAIMenu}
    bind:templateToEdit
    bind:templates
    bind:fileBrowserRemoteTerminalId
    bind:fileBrowserRemoteLabel
    {workflowBuilderQueuedEntry}
    {workflowBuilderQueuedEntries}
    bind:aiToolFlowConfig
    bind:recordingList
    {aggAvailable}
    {aggStatus}
    bind:showFolderManager
    bind:historyTrackingEnabled
    {updateChecking}
    {updateInfo}
    {updateDownloading}
    {updateProgress}
    {updateInstalled}
    {customFolders}
    bind:newFolderName
    {persistDefaultTerminalType}
    {addTerminal}
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
    onError={(msg) => { errorMessage = msg; }}
    onAggDownloadInfo={(info) => { aggDownloadInfo = info; }}
  />

  {#if !historyTrackingEnabled && !historyConsentDismissed}
    <div class="history-consent-banner">
      <p><strong>{$tr('appTerminals.historyConsentTitle')}</strong> — {$tr('appTerminals.historyConsentText')}</p>
      <div class="history-consent-actions">
        <button type="button" class="modal-primary-button" on:click={async () => { await SetHistoryTracking(true); historyTrackingEnabled = true; historyConsentDismissed = true; }}>{$tr('appTerminals.enable')}</button>
        <button type="button" class="modal-secondary-button" on:click={() => { historyConsentDismissed = true; }}>{$tr('appTerminals.notNow')}</button>
      </div>
    </div>
  {/if}

  <AppModals
    bind:showTemplatePicker
    {templates}
    {applyTemplate}
    bind:showWorkflowPicker
    {workflowPickerPlaybooks}
    {workflowPickerLoading}
    {runWorkflowFromPicker}
    closeWorkflowPicker={() => { showWorkflowPicker = false; }}
    bind:templatePromptState
    {closeTemplatePrompt}
    {submitTemplatePrompt}
    {handleTemplatePromptFieldChange}
    bind:aiPanelState
    {terminals}
    {closeAIPanel}
    onError={(msg) => { errorMessage = msg; }}
    bind:showAISettings
    bind:aiSettings
    bind:aiToolFlowConfig
    bind:aiToolFlowLists
    {aiProviders}
    promptPreview={getAIToolPromptPreview()}
    {closeAISettings}
    {saveAISettings}
    {applyAISettingsDefaults}
    {setDevOpsPrePromptExample}
    bind:showFunctionCatalog
    {functionCatalog}
    discoveryTerminalType={getDiscoveryTerminalType()}
    {queueWorkflowFromCatalog}
    closeFunctionCatalog={() => { showFunctionCatalog = false; }}
    bind:showSSHProfileModal
    {sshProfiles}
    {sshSecretBackend}
    {sshConnecting}
    {connectSSHProfile}
    closeSSHProfileModal={() => { showSSHProfileModal = false; }}
    onProfilesChanged={async (p) => { sshProfiles = Array.isArray(p) ? p : []; await loadSSHProfiles(); }}
    {hostKeyVerifyState}
    {acceptHostKey}
    {rejectHostKey}
    bind:aggDownloadInfo
    {downloadingAgg}
    cancelAggDownload={() => { aggDownloadInfo = null; }}
    {runAggDownload}
    bind:transcriptViewerState
    {closeTranscriptViewer}
  />
</main>
