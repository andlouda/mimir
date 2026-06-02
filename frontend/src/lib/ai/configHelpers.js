// Pure helpers for AI tool-flow config. State-free; safe to unit-test.

/** Normalizes a (possibly partial) AI tool-flow config to a fully-populated shape. */
export function normalizeAIToolFlowConfig(config) {
  const next = config || {};
  return {
    prompt: {
      prePrompt: next?.prompt?.prePrompt || '',
      requireStableToolId: next?.prompt?.requireStableToolId ?? true,
      includeRisk: next?.prompt?.includeRisk ?? true,
      includeCategory: next?.prompt?.includeCategory ?? true,
      includeTerminalOutput: next?.prompt?.includeTerminalOutput ?? true,
      maxTerminalContext: Number(next?.prompt?.maxTerminalContext) || 12000,
      allowTemplateNameFallback: next?.prompt?.allowTemplateNameFallback ?? false,
    },
    toolFilter: {
      includeCategories: Array.isArray(next?.toolFilter?.includeCategories) ? next.toolFilter.includeCategories : [],
      excludeCategories: Array.isArray(next?.toolFilter?.excludeCategories) ? next.toolFilter.excludeCategories : [],
      includeToolIds: Array.isArray(next?.toolFilter?.includeToolIds) ? next.toolFilter.includeToolIds : [],
      excludeToolIds: Array.isArray(next?.toolFilter?.excludeToolIds) ? next.toolFilter.excludeToolIds : [],
    },
    approval: {
      respectStepFlag: next?.approval?.respectStepFlag ?? true,
      requireApprovalForLow: next?.approval?.requireApprovalForLow ?? false,
      requireApprovalForMedium: next?.approval?.requireApprovalForMedium ?? true,
      requireApprovalForHigh: next?.approval?.requireApprovalForHigh ?? true,
    },
    execution: {
      enabled: next?.execution?.enabled ?? true,
      workflowMode: ['assist', 'approve', 'auto'].includes(next?.execution?.workflowMode) ? next.execution.workflowMode : 'approve',
      workflowIdPrefix: next?.execution?.workflowIdPrefix || 'ai-tool-run',
      workflowName: next?.execution?.workflowName || 'AI Tool Run',
      forceRequiresApproval: next?.execution?.forceRequiresApproval ?? false,
    },
  };
}

/** Joins a string array into a comma-separated text field value. */
export function listToText(values) {
  return Array.isArray(values) ? values.join(', ') : '';
}

/** Splits a comma-separated text field value into a trimmed, non-empty list. */
export function textToList(value) {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}
