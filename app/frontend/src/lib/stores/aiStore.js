import { writable } from 'svelte/store';

export const defaultAISettings = {
  provider: 'openai',
  model: 'gpt-5.4-mini',
  baseUrl: '',
  apiKey: '',
};

export const defaultAIToolFlowConfig = {
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

export const defaultAIToolFlowLists = {
  includeCategories: '',
  excludeCategories: '',
  includeToolIds: '',
  excludeToolIds: '',
};

export const immutableAIToolGuardrails = [
  'System guardrails (non-editable):',
  '1. Use only registered tool IDs from the provided list.',
  '2. Never invent raw shell commands and never use name-based fallback.',
  '3. Only choose read-only, diagnostic tools.',
  '4. Never choose tools that write, delete, install, restart, stop, kill, deploy, patch, scale, prune, or otherwise mutate files, packages, services, containers, clusters, firewall rules, or system state.',
  '5. Never access, print, or exfiltrate secrets, tokens, API keys, SSH keys, environment credentials, or credential stores.',
  '6. If the request requires mutation, secret access, or anything ambiguous, return an empty toolId and explain the refusal.',
].join('\n');

export const devopsPrePromptExample = 'You are a cautious DevOps terminal assistant. Prefer read-only diagnostics first, summarize likely impact before suggesting changes, and choose the least disruptive tool that helps inspect Kubernetes, Docker, networking, logs, processes, and system health. Escalate to mutating actions only when the user clearly asks for them.';

export const aiPanelState = writable(null);
export const showFunctionCatalog = writable(false);
export const functionCatalog = writable([]);
export const showAISettings = writable(false);
export const aiProviders = writable([]);
export const aiSettings = writable({ ...defaultAISettings });
export const aiToolFlowConfig = writable({
  ...defaultAIToolFlowConfig,
  prompt: { ...defaultAIToolFlowConfig.prompt },
  toolFilter: { ...defaultAIToolFlowConfig.toolFilter },
  approval: { ...defaultAIToolFlowConfig.approval },
  execution: { ...defaultAIToolFlowConfig.execution },
});
export const aiToolFlowLists = writable({ ...defaultAIToolFlowLists });
