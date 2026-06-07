import { get } from 'svelte/store';
import { normalizeAIToolFlowConfig, listToText, textToList } from '../ai/configHelpers.js';
import {
  aiProviders,
  aiSettings,
  aiToolFlowConfig,
  aiToolFlowLists,
  devopsPrePromptExample,
  immutableAIToolGuardrails,
  showAISettings,
} from '../stores/aiStore.js';
import { errorMessage, showAIMenu } from '../stores/uiStore.js';

function app() {
  return window['go']['main']['App'];
}

export function syncAIToolFlowListsFromConfig() {
  const config = get(aiToolFlowConfig);
  aiToolFlowLists.set({
    includeCategories: listToText(config.toolFilter.includeCategories),
    excludeCategories: listToText(config.toolFilter.excludeCategories),
    includeToolIds: listToText(config.toolFilter.includeToolIds),
    excludeToolIds: listToText(config.toolFilter.excludeToolIds),
  });
}

export function applyAIToolFlowListsToConfig() {
  const config = get(aiToolFlowConfig);
  const lists = get(aiToolFlowLists);
  aiToolFlowConfig.set({
    ...config,
    prompt: {
      ...config.prompt,
      maxTerminalContext: Number(config.prompt.maxTerminalContext) || 12000,
    },
    toolFilter: {
      includeCategories: textToList(lists.includeCategories),
      excludeCategories: textToList(lists.excludeCategories),
      includeToolIds: textToList(lists.includeToolIds),
      excludeToolIds: textToList(lists.excludeToolIds),
    },
  });
}

export function getEffectiveAIToolPrePrompt() {
  const configured = get(aiToolFlowConfig)?.prompt?.prePrompt?.trim();
  return configured || 'You are a terminal automation assistant.';
}

export function getAIToolPromptPreview() {
  const config = get(aiToolFlowConfig);
  const stableIdInstruction = 'Use only the provided tools and refer to them by the exact stable tool id.';
  const selectionShape = '{"toolId":"...","variables":{"Param":"value"},"reason":"..."}';

  return [
    getEffectiveAIToolPrePrompt(),
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
    config.prompt.includeTerminalOutput ? '<trimmed terminal output>' : '<omitted by config>',
    '',
    'User goal:',
    '<goal>',
    '',
    'Available tools:',
    '<tool list derived from current filters>',
  ].join('\n');
}

export function setDevOpsPrePromptExample() {
  const config = get(aiToolFlowConfig);
  aiToolFlowConfig.set({
    ...config,
    prompt: {
      ...config.prompt,
      prePrompt: devopsPrePromptExample,
    },
  });
}

export function getEditablePromptIntroPreview() {
  const configured = get(aiToolFlowConfig)?.prompt?.prePrompt?.trim();
  return configured || devopsPrePromptExample;
}

export function isUsingDefaultPromptIntroPreview() {
  return !get(aiToolFlowConfig)?.prompt?.prePrompt?.trim();
}

export async function loadAIProviders() {
  try {
    const raw = await app()['GetAIProvidersJSON']();
    aiProviders.set(JSON.parse(raw));
  } catch (error) {
    console.error('Failed to load AI providers:', error);
    aiProviders.set([]);
  }
}

export async function loadAISettingsConfig() {
  aiSettings.set(JSON.parse(await app()['GetAISettingsJSON']()));
  aiToolFlowConfig.set(normalizeAIToolFlowConfig(JSON.parse(await app()['GetAIToolFlowConfigJSON']())));
  syncAIToolFlowListsFromConfig();
}

export async function openAISettings() {
  await loadAIProviders();
  syncAIToolFlowListsFromConfig();
  showAISettings.set(true);
  showAIMenu.set(false);
}

export function closeAISettings() {
  showAISettings.set(false);
}

export function toggleAIMenu() {
  showAIMenu.update((value) => !value);
}

export function applyAISettingsDefaults(provider) {
  const providers = get(aiProviders);
  const settings = get(aiSettings);
  const desc = providers.find((p) => p.id === provider);
  if (!desc) return;

  const otherModelDefaults = providers.filter((p) => p.id !== provider).map((p) => p.defaultModel).filter(Boolean);
  const otherUrlDefaults = providers.filter((p) => p.id !== provider).map((p) => p.defaultBaseUrl).filter(Boolean);
  aiSettings.set({
    ...settings,
    model: !settings.model || otherModelDefaults.includes(settings.model) ? desc.defaultModel : settings.model,
    baseUrl: !settings.baseUrl || otherUrlDefaults.includes(settings.baseUrl) ? desc.defaultBaseUrl : settings.baseUrl,
  });
}

export async function saveAISettings() {
  try {
    applyAIToolFlowListsToConfig();
    const saved = JSON.parse(await app()['UpdateAISettingsJSON'](JSON.stringify(get(aiSettings))));
    aiSettings.set({ ...saved });
    const savedFlowConfig = JSON.parse(await app()['UpdateAIToolFlowConfigJSON'](JSON.stringify(get(aiToolFlowConfig))));
    aiToolFlowConfig.set(normalizeAIToolFlowConfig(savedFlowConfig));
    syncAIToolFlowListsFromConfig();
    showAISettings.set(false);
    errorMessage.set('');
  } catch (error) {
    errorMessage.set(`Failed to save AI settings: ${error.message || error}`);
  }
}
