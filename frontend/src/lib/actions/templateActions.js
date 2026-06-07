import { get } from 'svelte/store';
import { ApplyTemplate } from '../../../wailsjs/go/main/App';
import { t as tr } from '../i18n.js';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
import {
  templates,
  templatePromptState,
  showWorkflowPicker,
  workflowPickerLoading,
  workflowPickerPlaybooks,
} from '../stores/templateStore.js';
import { errorMessage } from '../stores/uiStore.js';
import { buildPromptLabel, extractTemplateVariables, normalizeTemplates } from '../templates/templateHelpers';

function app() {
  return window['go']['main']['App'];
}

export function loadTemplatesFromBackend() {
  return app()['GetTemplates']().then((nextTemplates) => {
    templates.set(normalizeTemplates(nextTemplates));
    errorMessage.set('');
  });
}

export function templateNameFromTool(tool) {
  return String(tool || '').replace(/^template:/, '');
}

export function workflowStepCommand(template, terminalType) {
  if (!template?.commands) return '';
  return template.commands[terminalType] || (terminalType === 'ssh' ? template.commands.bash : '') || '';
}

export function workflowPromptVariables(fields) {
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
  app()['RunDiscoveryJSON'](
    field.discoveryTool,
    terminalType,
    JSON.stringify(workflowPromptVariables(state.fields))
  )
    .then((raw) => {
      const values = JSON.parse(raw);
      field.suggestions = Array.isArray(values) ? values : [];
      field.loadingSuggestions = false;
      templatePromptState.set(get(templatePromptState));
    })
    .catch((error) => {
      field.suggestions = [];
      field.loadingSuggestions = false;
      field.suggestionError = error.message || String(error);
      templatePromptState.set(get(templatePromptState));
    });
}

export function openTemplatePrompt(template, terminalType, terminalId, variableNames) {
  const paramMap = {};
  for (const parameter of (template.parameters || [])) {
    paramMap[parameter.name] = parameter;
  }

  const nextState = {
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
    })),
  };
  templatePromptState.set(nextState);

  for (const field of nextState.fields) {
    if (!field.discoveryTool) continue;
    field.loadingSuggestions = true;
    app()['RunDiscoveryJSON'](field.discoveryTool, terminalType, '{}')
      .then((raw) => {
        const values = JSON.parse(raw);
        field.suggestions = Array.isArray(values) ? values : [];
        field.loadingSuggestions = false;
        templatePromptState.set(get(templatePromptState));
      })
      .catch(() => {
        field.loadingSuggestions = false;
        templatePromptState.set(get(templatePromptState));
      });
  }
}

export function buildWorkflowPromptFields(playbook, terminalType) {
  const fields = [];
  const currentTemplates = get(templates);

  for (const [stepIndex, step] of (playbook.steps || []).entries()) {
    if (step.type !== 'run_tool' || !step.tool) continue;
    const templateName = templateNameFromTool(step.tool);
    const template = templateName ? currentTemplates.find((item) => item.name === templateName) : null;
    if (!template) continue;

    const command = workflowStepCommand(template, terminalType);
    const variableNames = command ? extractTemplateVariables(command) : Object.keys(step.inputs || {});
    if (variableNames.length === 0) continue;

    const paramMap = {};
    for (const parameter of (template.parameters || [])) {
      paramMap[parameter.name] = parameter;
    }

    for (const name of variableNames) {
      fields.push({
        name: `${step.id}:${name}`,
        inputName: name,
        stepIndex,
        stepId: step.id,
        label: `${template.name} · ${buildPromptLabel(name)}`,
        value: step.inputs?.[name] || '',
        discoveryTool: paramMap[name]?.discoveryTool || '',
        suggestions: [],
        loadingSuggestions: false,
        suggestionError: '',
      });
    }
  }
  return fields;
}

export function openWorkflowPrompt(playbook, activeTerminal) {
  const fields = buildWorkflowPromptFields(playbook, activeTerminal.type);
  if (fields.length === 0) return false;

  const nextState = {
    kind: 'workflow',
    templateName: playbook.name,
    workflowPlaybook: playbook,
    terminalType: activeTerminal.type,
    terminalId: activeTerminal.id,
    terminalName: activeTerminal.name || '',
    terminalOutput: activeTerminal.outputBuffer || '',
    fields,
  };
  templatePromptState.set(nextState);

  for (const field of nextState.fields) {
    loadPromptFieldSuggestions(field, nextState, activeTerminal.type);
  }
  return true;
}

export function closeTemplatePrompt() {
  templatePromptState.set(null);
}

export function handleTemplatePromptFieldChange(changedField) {
  const state = get(templatePromptState);
  if (!state || state.kind !== 'workflow') return;

  for (const field of state.fields || []) {
    if (field === changedField || !field.discoveryTool) continue;
    field.suggestions = [];
    loadPromptFieldSuggestions(field, state, state.terminalType || '');
  }
}

export async function runPromptedWorkflow(promptState, variables) {
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

  await app()['RunWorkflowDraftJSON'](
    definition,
    Number(promptState.terminalId),
    promptState.terminalType || '',
    promptState.terminalName || '',
    promptState.terminalOutput || ''
  );
}

export async function submitTemplatePrompt() {
  const state = get(templatePromptState);
  if (!state) return;

  const variables = {};
  for (const field of state.fields) {
    if (!field.value.trim()) {
      errorMessage.set(`Please enter a value for ${field.label}.`);
      return;
    }
    variables[field.name] = field.value.trim();
  }

  try {
    if (state.kind === 'workflow') {
      await runPromptedWorkflow(state, variables);
      errorMessage.set('');
      closeTemplatePrompt();
      return;
    }

    await app()['ApplyTemplateWithVariables'](
      state.terminalId,
      state.templateName,
      state.terminalType,
      variables
    );
    errorMessage.set('');
    closeTemplatePrompt();
  } catch (error) {
    errorMessage.set(`Failed to apply template: ${error.message || error}`);
  }
}

export async function applyTemplate(templateName) {
  const selectedId = get(activeTerminalId);
  if (selectedId === null) {
    errorMessage.set('Please select a terminal first.');
    return;
  }

  const activeTerminal = get(terminals).find((terminal) => terminal.id === selectedId);
  if (!activeTerminal) {
    errorMessage.set('Active terminal not found.');
    return;
  }

  const template = get(templates).find((item) => item.name === templateName);
  if (!template) {
    errorMessage.set(`Template '${templateName}' not found.`);
    return;
  }
  if (!template.commands) {
    errorMessage.set(`Template '${templateName}' has no 'commands' property.`);
    return;
  }

  const terminalType = activeTerminal.type;
  const commandToExecute = template.commands[terminalType] || (terminalType === 'ssh' ? template.commands.bash : null);
  if (!commandToExecute) {
    errorMessage.set(`Template '${templateName}' does not have a command for terminal type '${terminalType}'.`);
    return;
  }

  const requiredVariables = extractTemplateVariables(commandToExecute);
  if (requiredVariables.length > 0) {
    openTemplatePrompt(template, activeTerminal.type, activeTerminal.id, requiredVariables);
    return;
  }

  try {
    await ApplyTemplate(activeTerminal.id, templateName, activeTerminal.type);
    errorMessage.set('');
  } catch (error) {
    errorMessage.set(`Failed to apply template: ${error.message || error}`);
  }
}

export async function toggleWorkflowPicker() {
  if (get(showWorkflowPicker)) {
    showWorkflowPicker.set(false);
    return;
  }
  if (get(activeTerminalId) === null) {
    errorMessage.set(get(tr)('workflowPicker.noTerminal'));
    return;
  }

  showWorkflowPicker.set(true);
  workflowPickerLoading.set(true);
  try {
    const payload = await app()['GetPlaybooksJSON']();
    workflowPickerPlaybooks.set(JSON.parse(payload));
  } catch (error) {
    workflowPickerPlaybooks.set([]);
    errorMessage.set(`Failed to load workflows: ${error.message || error}`);
  } finally {
    workflowPickerLoading.set(false);
  }
}

export async function runWorkflowFromPicker(playbook) {
  const activeTerminal = get(terminals).find((terminal) => terminal.id === get(activeTerminalId));
  if (!activeTerminal) {
    errorMessage.set('Active terminal not found.');
    return;
  }
  if (openWorkflowPrompt(playbook, activeTerminal)) {
    errorMessage.set('');
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
    await app()['RunWorkflowDraftJSON'](
      definition,
      Number(activeTerminal.id),
      activeTerminal.type || '',
      activeTerminal.name || '',
      activeTerminal.outputBuffer || ''
    );
    errorMessage.set('');
  } catch (error) {
    errorMessage.set(`Failed to run workflow: ${error.message || error}`);
  }
}
