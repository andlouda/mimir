import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  applyTemplate,
  buildWorkflowPromptFields,
  closeTemplatePrompt,
  loadTemplatesFromBackend,
  openTemplatePrompt,
  runWorkflowFromPicker,
  submitTemplatePrompt,
  toggleWorkflowPicker,
  workflowPromptVariables,
} from './templateActions.js';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
import {
  showWorkflowPicker,
  templatePromptState,
  templates,
  workflowPickerLoading,
  workflowPickerPlaybooks,
} from '../stores/templateStore.js';
import { errorMessage } from '../stores/uiStore.js';
import { locale } from '../i18n.js';

vi.mock('../../../wailsjs/go/main/App', () => ({
  ApplyTemplate: vi.fn(),
}));

const { ApplyTemplate } = await import('../../../wailsjs/go/main/App');

beforeEach(() => {
  globalThis.localStorage = {
    getItem: vi.fn(),
    setItem: vi.fn(),
  };
  globalThis.window = {
    go: {
      main: {
        App: {
          ApplyTemplateWithVariables: vi.fn(),
          GetTemplates: vi.fn(),
          GetPlaybooksJSON: vi.fn(),
          RunDiscoveryJSON: vi.fn(),
          RunWorkflowDraftJSON: vi.fn(),
        },
      },
    },
  };
});

afterEach(() => {
  activeTerminalId.set(null);
  terminals.set([]);
  templates.set([]);
  templatePromptState.set(null);
  showWorkflowPicker.set(false);
  workflowPickerLoading.set(false);
  workflowPickerPlaybooks.set([]);
  errorMessage.set('');
  locale.set('en');
  vi.restoreAllMocks();
  delete globalThis.window;
  delete globalThis.localStorage;
});

describe('template actions', () => {
  test('applies a template without prompting when no variables are required', async () => {
    activeTerminalId.set(3);
    terminals.set([{ id: 3, type: 'bash', name: 'Bash' }]);
    templates.set([{ name: 'List', commands: { bash: 'ls -la' } }]);
    ApplyTemplate.mockResolvedValue(undefined);

    await applyTemplate('List');

    expect(ApplyTemplate).toHaveBeenCalledWith(3, 'List', 'bash');
    expect(get(errorMessage)).toBe('');
  });

  test('loads and normalizes templates from backend', async () => {
    window.go.main.App.GetTemplates.mockResolvedValue([{ name: 'Raw' }]);

    await loadTemplatesFromBackend();

    expect(get(templates)).toEqual([expect.objectContaining({
      name: 'Raw',
      description: '',
      commands: {},
      parameters: [],
    })]);
    expect(get(errorMessage)).toBe('');
  });

  test('opens a prompt for user-fillable template variables', async () => {
    activeTerminalId.set(4);
    terminals.set([{ id: 4, type: 'bash', name: 'Bash' }]);
    templates.set([{
      name: 'Tail',
      commands: { bash: 'tail -f {{ .LogPath }} {{ .CurrentDir }}' },
      parameters: [{ name: 'LogPath', discoveryTool: 'logs' }],
    }]);
    window.go.main.App.RunDiscoveryJSON.mockResolvedValue(JSON.stringify(['/var/log/app.log']));

    await applyTemplate('Tail');

    const state = get(templatePromptState);
    expect(state).toMatchObject({
      templateName: 'Tail',
      terminalType: 'bash',
      terminalId: 4,
    });
    expect(state.fields).toHaveLength(1);
    expect(state.fields[0]).toMatchObject({ name: 'LogPath', discoveryTool: 'logs' });
  });

  test('validates prompt values before submitting', async () => {
    templatePromptState.set({
      templateName: 'Tail',
      terminalType: 'bash',
      terminalId: 5,
      fields: [{ name: 'LogPath', label: 'Log Path', value: '   ' }],
    });

    await submitTemplatePrompt();

    expect(get(errorMessage)).toBe('Please enter a value for Log Path.');
    expect(window.go.main.App.ApplyTemplateWithVariables).not.toHaveBeenCalled();
  });

  test('submits template variables and closes prompt', async () => {
    templatePromptState.set({
      templateName: 'Tail',
      terminalType: 'bash',
      terminalId: 5,
      fields: [{ name: 'LogPath', label: 'Log Path', value: ' /var/log/app.log ' }],
    });

    await submitTemplatePrompt();

    expect(window.go.main.App.ApplyTemplateWithVariables).toHaveBeenCalledWith(
      5,
      'Tail',
      'bash',
      { LogPath: '/var/log/app.log' }
    );
    expect(get(templatePromptState)).toBe(null);
  });

  test('builds workflow prompt fields from template-backed steps', () => {
    templates.set([{
      name: 'Restart Service',
      commands: { bash: 'systemctl restart {{ .ServiceName }}' },
    }]);

    const fields = buildWorkflowPromptFields({
      steps: [{ id: 'step-1', type: 'run_tool', tool: 'template:Restart Service', inputs: {} }],
    }, 'bash');

    expect(fields).toEqual([expect.objectContaining({
      name: 'step-1:ServiceName',
      inputName: 'ServiceName',
      label: 'Restart Service · Service Name',
    })]);
    expect(workflowPromptVariables([{ inputName: 'ServiceName', value: 'nginx' }])).toEqual({ ServiceName: 'nginx' });
  });

  test('loads workflows when opening picker', async () => {
    activeTerminalId.set(1);
    window.go.main.App.GetPlaybooksJSON.mockResolvedValue(JSON.stringify([{ id: 'pb-1', name: 'Check' }]));

    await toggleWorkflowPicker();

    expect(get(showWorkflowPicker)).toBe(true);
    expect(get(workflowPickerLoading)).toBe(false);
    expect(get(workflowPickerPlaybooks)).toEqual([{ id: 'pb-1', name: 'Check' }]);
  });

  test('runs workflow immediately when no prompt fields are required', async () => {
    activeTerminalId.set(9);
    terminals.set([{ id: 9, type: 'bash', name: 'Ops', outputBuffer: 'recent output' }]);

    await runWorkflowFromPicker({ id: 'pb-2', name: 'Status', steps: [{ id: 's1', type: 'wait' }] });

    expect(window.go.main.App.RunWorkflowDraftJSON).toHaveBeenCalledWith(
      expect.stringContaining('"name":"Status"'),
      9,
      'bash',
      'Ops',
      'recent output'
    );
  });

  test('can open and close template prompt directly', () => {
    openTemplatePrompt({ name: 'Echo', parameters: [] }, 'bash', 2, ['Message']);
    expect(get(templatePromptState).fields[0]).toMatchObject({ name: 'Message', label: 'Message' });

    closeTemplatePrompt();
    expect(get(templatePromptState)).toBe(null);
  });
});
