import { afterEach, describe, expect, test } from 'vitest';
import { get } from 'svelte/store';
import {
  filteredTemplates,
  showTemplatePicker,
  templatePromptState,
  templateSearchQuery,
  templates,
  workflowPickerLoading,
  workflowPickerPlaybooks,
} from './templateStore.js';

afterEach(() => {
  templates.set([]);
  templateSearchQuery.set('');
  showTemplatePicker.set(false);
  workflowPickerLoading.set(false);
  workflowPickerPlaybooks.set([]);
  templatePromptState.set(null);
});

describe('template store', () => {
  test('filters normalized templates by name or description', () => {
    templates.set([
      { name: 'Docker PS', description: 'List containers', commands: { bash: 'docker ps' } },
      { name: 'Disk Usage', description: 'Inspect filesystem capacity', commands: { bash: 'df -h' } },
    ]);
    templateSearchQuery.set('filesystem');

    expect(get(filteredTemplates).map((template) => template.name)).toEqual(['Disk Usage']);
  });

  test('tracks picker and prompt state', () => {
    showTemplatePicker.set(true);
    workflowPickerLoading.set(true);
    workflowPickerPlaybooks.set([{ id: 'pb-1', name: 'Check' }]);
    templatePromptState.set({ templateName: 'Docker PS', fields: [] });

    expect(get(showTemplatePicker)).toBe(true);
    expect(get(workflowPickerLoading)).toBe(true);
    expect(get(workflowPickerPlaybooks)).toEqual([{ id: 'pb-1', name: 'Check' }]);
    expect(get(templatePromptState)).toEqual({ templateName: 'Docker PS', fields: [] });
  });
});
