import { derived, writable } from 'svelte/store';
import { normalizeTemplates } from '../templates/templateHelpers.js';

export const templates = writable([]);
export const templateToEdit = writable(null);
export const templatePromptState = writable(null);
export const showTemplatePicker = writable(false);
export const showWorkflowPicker = writable(false);
export const workflowPickerPlaybooks = writable([]);
export const workflowPickerLoading = writable(false);
export const templateSearchQuery = writable('');

export const filteredTemplates = derived(
  [templates, templateSearchQuery],
  ([$templates, $templateSearchQuery]) => {
    const query = $templateSearchQuery.toLowerCase();
    return normalizeTemplates($templates).filter((template) =>
      template.name.toLowerCase().includes(query) ||
      template.description.toLowerCase().includes(query)
    );
  },
);
