import { get } from 'svelte/store';
import { notesPanelOpen } from '../stores/uiStore.js';
import { showTemplatePicker, showWorkflowPicker } from '../stores/templateStore.js';
import { terminals } from '../stores/terminalStore.js';

export function createKeydownHandler({
  handleResize,
  toggleTerminalSearch,
  toggleWorkflowPicker,
  closeTerminalSearch,
  schedule = setTimeout,
} = {}) {
  return function handleGlobalKeydown(event) {
    if (event.ctrlKey && event.shiftKey && event.key === 'N') {
      event.preventDefault();
      notesPanelOpen.update((open) => !open);
      schedule(handleResize, 50);
      return;
    }

    if (event.ctrlKey && event.shiftKey && event.key === 'F') {
      event.preventDefault();
      toggleTerminalSearch();
      return;
    }

    if (event.ctrlKey && event.shiftKey && (event.key === 'P' || event.key === 'p')) {
      event.preventDefault();
      showTemplatePicker.update((open) => !open);
      return;
    }

    if (event.ctrlKey && event.shiftKey && (event.key === 'W' || event.key === 'w')) {
      event.preventDefault();
      toggleWorkflowPicker();
      return;
    }

    if (event.key === 'Escape') {
      if (get(showTemplatePicker)) {
        showTemplatePicker.set(false);
        return;
      }
      if (get(showWorkflowPicker)) {
        showWorkflowPicker.set(false);
        return;
      }
      const visibleSearches = get(terminals).filter((terminal) => terminal.searchVisible);
      for (const terminal of visibleSearches) {
        closeTerminalSearch(terminal.id);
      }
    }
  };
}
