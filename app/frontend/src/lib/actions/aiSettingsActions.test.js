import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  aiProviders,
  aiSettings,
  aiToolFlowConfig,
  aiToolFlowLists,
  defaultAISettings,
  defaultAIToolFlowConfig,
  defaultAIToolFlowLists,
  showAISettings,
} from '../stores/aiStore.js';
import { errorMessage, showAIMenu } from '../stores/uiStore.js';
import {
  applyAISettingsDefaults,
  getAIToolPromptPreview,
  loadAISettingsConfig,
  openAISettings,
  saveAISettings,
  setDevOpsPrePromptExample,
  syncAIToolFlowListsFromConfig,
} from './aiSettingsActions.js';

beforeEach(() => {
  globalThis.window = {
    go: {
      main: {
        App: {
          GetAIProvidersJSON: vi.fn(),
          GetAISettingsJSON: vi.fn(),
          GetAIToolFlowConfigJSON: vi.fn(),
          UpdateAISettingsJSON: vi.fn(),
          UpdateAIToolFlowConfigJSON: vi.fn(),
        },
      },
    },
  };
});

afterEach(() => {
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
  showAISettings.set(false);
  showAIMenu.set(false);
  errorMessage.set('');
  vi.clearAllMocks();
  delete globalThis.window;
});

describe('ai settings actions', () => {
  test('syncs tool filter lists from config', () => {
    aiToolFlowConfig.update((config) => ({
      ...config,
      toolFilter: { ...config.toolFilter, includeCategories: ['Docker'], excludeToolIds: ['danger'] },
    }));

    syncAIToolFlowListsFromConfig();

    expect(get(aiToolFlowLists).includeCategories).toBe('Docker');
    expect(get(aiToolFlowLists).excludeToolIds).toBe('danger');
  });

  test('loads settings and flow config from backend', async () => {
    window.go.main.App.GetAISettingsJSON.mockResolvedValue(JSON.stringify({ provider: 'ollama', model: 'qwen', baseUrl: 'http://localhost', apiKey: '' }));
    window.go.main.App.GetAIToolFlowConfigJSON.mockResolvedValue(JSON.stringify(defaultAIToolFlowConfig));

    await loadAISettingsConfig();

    expect(get(aiSettings).provider).toBe('ollama');
    expect(get(aiToolFlowConfig).prompt.requireStableToolId).toBe(true);
  });

  test('opens settings after loading providers', async () => {
    window.go.main.App.GetAIProvidersJSON.mockResolvedValue(JSON.stringify([{ id: 'openai', defaultModel: 'gpt', defaultBaseUrl: 'https://api' }]));
    showAIMenu.set(true);

    await openAISettings();

    expect(get(aiProviders)).toHaveLength(1);
    expect(get(showAISettings)).toBe(true);
    expect(get(showAIMenu)).toBe(false);
  });

  test('applies provider defaults without clearing custom values', () => {
    aiProviders.set([
      { id: 'openai', defaultModel: 'gpt-5', defaultBaseUrl: 'https://api.openai.test' },
      { id: 'ollama', defaultModel: 'qwen', defaultBaseUrl: 'http://localhost:11434' },
    ]);
    aiSettings.set({ provider: 'ollama', model: 'gpt-5', baseUrl: 'https://custom.test', apiKey: 'k' });

    applyAISettingsDefaults('ollama');

    expect(get(aiSettings).model).toBe('qwen');
    expect(get(aiSettings).baseUrl).toBe('https://custom.test');
  });

  test('builds prompt preview from config', () => {
    setDevOpsPrePromptExample();

    expect(getAIToolPromptPreview()).toContain('Use only registered tool IDs');
    expect(getAIToolPromptPreview()).toContain('Available tools');
  });

  test('saves settings and flow config', async () => {
    window.go.main.App.UpdateAISettingsJSON.mockImplementation(async (payload) => payload);
    window.go.main.App.UpdateAIToolFlowConfigJSON.mockImplementation(async (payload) => payload);
    showAISettings.set(true);

    await saveAISettings();

    expect(window.go.main.App.UpdateAISettingsJSON).toHaveBeenCalled();
    expect(window.go.main.App.UpdateAIToolFlowConfigJSON).toHaveBeenCalled();
    expect(get(showAISettings)).toBe(false);
    expect(get(errorMessage)).toBe('');
  });
});
