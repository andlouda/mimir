import { writable } from 'svelte/store';

function initialNotesPanelWidth() {
  try {
    return parseInt(localStorage.getItem('mimir-notes-width') || '380');
  } catch {
    return 380;
  }
}

export const currentPage = writable('terminals');
// tmux integration for new terminals: 'invisible' | 'classic' | 'off'
// (loaded from the backend at start-up; see terminal/tmux_options.go).
export const tmuxIntegrationMode = writable('invisible');
export const errorMessage = writable('');
export const showAIMenu = writable(false);
export const notesPanelOpen = writable(false);
export const notesPanelWidth = writable(initialNotesPanelWidth());
export const showFolderManager = writable(false);
export const historyTrackingEnabled = writable(false);
export const historyConsentDismissed = writable(false);
export const transcriptViewerState = writable(null);
// When set to { terminalId, terminalType, label } the secure .env viewer modal opens.
export const dotEnvViewerState = writable(null);
