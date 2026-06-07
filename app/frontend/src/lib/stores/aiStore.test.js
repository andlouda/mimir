import { afterEach, describe, expect, test } from 'vitest';
import { get } from 'svelte/store';
import {
  aiPanelState,
  aiProviders,
  aiSettings,
  aiToolFlowConfig,
  aiToolFlowLists,
  defaultAISettings,
  defaultAIToolFlowConfig,
  defaultAIToolFlowLists,
  functionCatalog,
  immutableAIToolGuardrails,
  showAISettings,
  showFunctionCatalog,
} from './aiStore.js';

afterEach(() => {
  aiPanelState.set(null);
  showFunctionCatalog.set(false);
  functionCatalog.set([]);
  showAISettings.set(false);
  aiProviders.set([]);
  aiSettings.set({ ...defaultAISettings });
  aiToolFlowConfig.set({
    ...defaultAIToolFlowConfig,
    prompt: { ...defaultAIToolFlowConfig.prompt },
    toolFilter: { ...defaultAIToolFlowConfig.toolFilter },
    approval: { ...defaultAIToolFlowConfig.approval },
    execution: { ...defaultAIToolFlowConfig.execution },
  });
  aiToolFlowLists.set({ ...defaultAIToolFlowLists });
});

describe('ai store', () => {
  test('exposes guarded AI defaults', () => {
    expect(get(aiSettings)).toMatchObject({ provider: 'openai', model: 'gpt-5.4-mini' });
    expect(get(aiToolFlowConfig).prompt.requireStableToolId).toBe(true);
    expect(immutableAIToolGuardrails).toContain('Use only registered tool IDs');
  });

  test('tracks modal and catalog state', () => {
    aiPanelState.set({ mode: 'explain_output', terminalId: 1 });
    showAISettings.set(true);
    showFunctionCatalog.set(true);
    functionCatalog.set([{ id: 'ai:explain_output', name: 'Explain Output' }]);
    aiProviders.set([{ id: 'ollama', label: 'Ollama' }]);
    aiToolFlowLists.set({ includeCategories: 'Docker', excludeCategories: '', includeToolIds: '', excludeToolIds: '' });

    expect(get(aiPanelState).mode).toBe('explain_output');
    expect(get(showAISettings)).toBe(true);
    expect(get(showFunctionCatalog)).toBe(true);
    expect(get(functionCatalog)).toHaveLength(1);
    expect(get(aiProviders)[0].id).toBe('ollama');
    expect(get(aiToolFlowLists).includeCategories).toBe('Docker');
  });
});
