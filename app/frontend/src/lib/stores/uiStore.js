import { writable } from 'svelte/store';

function initialNotesPanelWidth() {
  try {
    return parseInt(localStorage.getItem('mimir-notes-width') || '380');
  } catch {
    return 380;
  }
}

export const currentPage = writable('terminals');
export const errorMessage = writable('');
export const showAIMenu = writable(false);
export const notesPanelOpen = writable(false);
export const notesPanelWidth = writable(initialNotesPanelWidth());
export const showFolderManager = writable(false);
export const historyTrackingEnabled = writable(false);
export const historyConsentDismissed = writable(false);
export const transcriptViewerState = writable(null);
