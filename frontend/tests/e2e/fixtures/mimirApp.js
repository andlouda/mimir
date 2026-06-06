import { expect } from '@playwright/test';

export const mockPlaybooks = [
  {
    id: 'docker-compose-debug',
    name: 'Docker Compose Debug',
    description: 'Inspect compose services, logs and health without mutating state.',
    mode: 'assist',
    protected: true,
    steps: [
      {
        id: 'discover-services',
        type: 'run_discovery',
        discoveryTool: 'docker_compose_services',
        inputs: {},
      },
      {
        id: 'explain',
        type: 'ask_ai',
        prompt: 'Summarize likely failure causes from the collected diagnostics.',
        inputs: {},
      },
    ],
  },
  {
    id: 'k8s-pod-triage',
    name: 'Kubernetes Pod Triage',
    description: 'Collect pod context, describe output and logs for read-only triage.',
    mode: 'approve',
    protected: true,
    steps: [
      {
        id: 'discover-pods',
        type: 'run_discovery',
        discoveryTool: 'k8s_get_pods',
        inputs: {},
      },
    ],
  },
  {
    id: 'approval-drill',
    name: 'Approval Drill',
    description: 'Exercise pending approval handling for guarded workflow steps.',
    mode: 'approve',
    protected: true,
    steps: [
      {
        id: 'restart-service',
        type: 'run_tool',
        tool: 'template:Restart Service',
        inputs: { ServiceName: 'api' },
      },
      {
        id: 'explain',
        type: 'ask_ai',
        prompt: 'Explain whether the approved action completed.',
        inputs: {},
      },
    ],
  },
];

export const mockFunctionCatalog = [
  {
    id: 'docker_compose_services',
    name: 'Docker Compose Services',
    description: 'List docker compose services for the active project.',
    category: 'docker',
    kind: 'run_discovery',
    risk: 'low',
    parameters: [],
  },
  {
    id: 'explain_output',
    name: 'Explain Output',
    description: 'Ask AI to explain collected terminal or tool output.',
    category: 'ai',
    kind: 'ask_ai',
    risk: 'low',
    parameters: [
      { name: 'prompt', required: false },
    ],
  },
];

const defaultAISettings = {
  provider: 'openai',
  model: 'gpt-5.4-mini',
  baseUrl: '',
  apiKey: '',
};

const defaultAIToolFlowConfig = {
  prompt: {
    prePrompt: '',
    requireStableToolId: true,
    includeRisk: true,
    includeCategory: true,
    includeTerminalOutput: true,
    maxTerminalContext: 12000,
    allowTemplateNameFallback: false,
  },
  toolFilter: {
    includeCategories: [],
    excludeCategories: [],
    includeToolIds: [],
    excludeToolIds: [],
  },
  approval: {
    requireLowRiskApproval: false,
    requireMediumRiskApproval: true,
    requireHighRiskApproval: true,
  },
  execution: {
    mode: 'assist',
  },
};

function completedWorkflowState(workflowId = 'draft') {
  return {
    workflowId,
    status: 'completed',
    pendingApproval: null,
    discovery: {
      'discover-services': ['api', 'db'],
    },
    events: [
      {
        type: 'workflow_started',
        stepId: '',
        message: 'Workflow started.',
        metadata: {},
      },
      {
        type: 'step_completed',
        stepId: 'discover-services',
        message: 'Discovery completed.',
        metadata: { values: '2' },
      },
    ],
  };
}

export async function installMimirMocks(page) {
  await page.addInitScript(({ playbooks, functionCatalog, aiSettings, aiToolFlowConfig }) => {
    localStorage.setItem('mimir-locale', 'en');

    let nextTerminalId = 1;
    let savedPlaybooks = [...playbooks];
    const noop = () => {};
    const asyncNoop = async () => {};
    const completedWorkflowState = (workflowId = 'draft') => ({
      workflowId,
      status: 'completed',
      pendingApproval: null,
      discovery: {
        'discover-services': ['api', 'db'],
      },
      events: [
        {
          type: 'workflow_started',
          stepId: '',
          message: 'Workflow started.',
          metadata: {},
        },
        {
          type: 'step_completed',
          stepId: 'discover-services',
          message: 'Discovery completed.',
          metadata: { values: '2' },
        },
      ],
    });
    const pendingApprovalState = (workflowId = 'approval-drill') => ({
      workflowId,
      status: 'pending_approval',
      stepIndex: 0,
      outputs: {},
      discovery: {},
      pendingApproval: {
        stepId: 'restart-service',
        toolId: 'template:Restart Service',
        toolName: 'Restart Service',
        risk: 'high',
        reason: 'high-risk tools require approval',
      },
      events: [
        {
          type: 'step_pending_approval',
          stepId: 'restart-service',
          message: 'high-risk tools require approval',
          metadata: { tool_id: 'template:Restart Service', risk: 'high' },
        },
        {
          type: 'workflow_paused',
          stepId: 'restart-service',
          message: 'approval required for step restart-service',
          metadata: {},
        },
      ],
    });
    const deniedApprovalState = (rawState) => {
      const state = JSON.parse(rawState);
      return {
        ...state,
        pendingApproval: null,
        events: [
          ...(state.events || []),
          {
            type: 'approval_denied',
            stepId: state.pendingApproval?.stepId || 'restart-service',
            message: 'approval denied by user',
            metadata: { risk: state.pendingApproval?.risk || 'high' },
          },
        ],
      };
    };
    const runWorkflow = async (rawDefinition) => {
      const definition = JSON.parse(rawDefinition || '{}');
      if (definition.id === 'approval-drill') {
        return JSON.stringify(pendingApprovalState(definition.id));
      }
      return JSON.stringify(completedWorkflowState(definition.id || 'draft'));
    };

    window.runtime = {
      EventsOnMultiple: () => noop,
      EventsOff: noop,
      EventsEmit: noop,
      LogPrint: noop,
      LogTrace: noop,
      LogDebug: noop,
      LogInfo: noop,
      LogWarning: noop,
      LogError: noop,
      LogFatal: noop,
      BrowserOpenURL: noop,
    };

    window.go = {
      main: {
        App: {
          AcceptSSHHostKey: asyncNoop,
          AppendTerminalTranscript: asyncNoop,
          ApplyTemplate: asyncNoop,
          ApplyTemplateWithVariables: asyncNoop,
          CheckForUpdates: async () => JSON.stringify({
            configured: true,
            currentVersion: '0.2.0',
            latestVersion: '0.2.0',
            updateAvailable: false,
            platform: 'linux-amd64',
            releaseURL: 'https://example.invalid/releases/v0.2.0',
          }),
          CloseSSHTerminalFull: asyncNoop,
          CloseTerminal: asyncNoop,
          ConfirmFrontendReady: asyncNoop,
          DeletePlaybook: async () => JSON.stringify(savedPlaybooks),
          DeleteRecording: asyncNoop,
          DeleteTerminalFolder: async () => [],
          DeleteTemplate: asyncNoop,
          DownloadAgg: asyncNoop,
          DownloadUpdate: asyncNoop,
          ExportRecordingGIF: async () => '/tmp/mimir.gif',
          ExportRecordingScrubbed: async () => '/tmp/mimir.cast',
          ExportRecordingTrimmed: async () => '/tmp/mimir-trimmed.cast',
          ExportRecordingTrimmedGIF: async () => '/tmp/mimir-trimmed.gif',
          GetAggDownloadInfo: async () => ({ url: 'https://example.invalid/agg', destination: '/tmp/agg', platform: 'linux-amd64' }),
          GetAggStatus: async () => 'missing',
          GetAIProvidersJSON: async () => JSON.stringify([{ id: 'openai', name: 'OpenAI' }]),
          GetAISettingsJSON: async () => JSON.stringify(aiSettings),
          GetAIToolFlowConfigJSON: async () => JSON.stringify(aiToolFlowConfig),
          GetAvailableTerminalTypes: async () => [{ value: 'bash', label: 'Bash' }, { value: 'zsh', label: 'Zsh' }, { value: 'ssh', label: 'SSH' }],
          GetFunctionCatalogJSON: async () => JSON.stringify(functionCatalog),
          GetLoadedSessionData: async () => ({ terminals: [] }),
          GetPendingUpdate: async () => '',
          GetPlaybooksJSON: async () => JSON.stringify(savedPlaybooks),
          GetRecording: async () => ({ id: 'rec-1', title: 'Recording', content: '' }),
          GetSSHProfiles: async () => [],
          GetSSHSecretBackend: async () => 'file',
          GetSSHTerminalLabel: async () => '',
          GetTemplates: async () => [],
          GetTerminalFolders: async () => [],
          GetTerminalTmuxStatus: async () => ({ active: false, sessionName: '', mode: '', status: '' }),
          GetTerminalTranscriptExcerpt: async () => '',
          GetTerminalTranscriptFull: async () => 'mocked transcript body — first line\nsecond line\nthird line\n',
          GetTerminalTranscriptContent: async (resumeId) => ({
            resumeId,
            text: 'mocked transcript body — first line\nsecond line\nthird line\n',
            size: 64,
            readBytes: 64,
            truncated: false,
          }),
          ListTranscripts: async () => ([
            { resumeId: 'sample-resume-1', name: 'API host', type: 'ssh', sshProfileId: 'prod-api', size: 1024, modTime: new Date().toISOString() },
            { resumeId: 'sample-resume-2', name: 'Local shell', type: 'bash', size: 256, modTime: new Date(Date.now() - 3600000).toISOString() },
          ]),
          SaveTranscriptMetadata: asyncNoop,
          InitializeTerminal: asyncNoop,
          IsAggInstalled: async () => false,
          IsHistoryTrackingEnabled: async () => false,
          KillTmuxSession: asyncNoop,
          ListRecordings: async () => [],
          ReconnectSSHTerminal: async (id) => id,
          RejectSSHHostKey: asyncNoop,
          RemoveTerminalState: asyncNoop,
          RestartToApplyUpdate: asyncNoop,
          RejectWorkflowDraftJSON: async (rawState) => JSON.stringify(deniedApprovalState(rawState)),
          ResumeWorkflowDraftJSON: async (rawDefinition) => {
            const definition = JSON.parse(rawDefinition || '{}');
            return JSON.stringify({
              ...completedWorkflowState(definition.id || 'draft'),
              events: [
                {
                  type: 'approval_granted',
                  stepId: 'restart-service',
                  message: 'high-risk tools require approval',
                  metadata: { risk: 'high' },
                },
                ...completedWorkflowState(definition.id || 'draft').events,
              ],
            });
          },
          RunWorkflowDraftJSON: runWorkflow,
          SaveCurrentSession: asyncNoop,
          SavePlaybookJSON: async (rawDefinition) => {
            const definition = JSON.parse(rawDefinition);
            const saved = { ...definition, protected: false };
            savedPlaybooks = [
              ...savedPlaybooks.filter((playbook) => playbook.id !== saved.id),
              saved,
            ];
            return JSON.stringify(saved);
          },
          SaveTerminalFolder: async () => [],
          SetHistoryTracking: asyncNoop,
          StartRecording: asyncNoop,
          StartSSHTerminal: async () => nextTerminalId++,
          StartTerminal: async () => nextTerminalId++,
          StartTerminalWithOptions: async () => nextTerminalId++,
          StopRecording: asyncNoop,
          ToggleFavorite: asyncNoop,
          UpdateTerminalFolder: async () => [],
          UpdateTerminalState: asyncNoop,
          WriteToTerminal: asyncNoop,
        },
      },
    };
  }, {
    playbooks: mockPlaybooks,
    functionCatalog: mockFunctionCatalog,
    aiSettings: defaultAISettings,
    aiToolFlowConfig: defaultAIToolFlowConfig,
  });
}

export async function openApp(page) {
  await installMimirMocks(page);
  await page.goto('/');
  await expect(page.locator('.brand-text')).toHaveText('Mimir');
}
