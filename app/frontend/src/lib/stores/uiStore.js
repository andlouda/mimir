import { writable } from 'svelte/store';

function initialNotesPanelWidth() {
  try {
    return parseInt(localStorage.getItem('mimir-notes-width') || '380');
  } catch {
    return 380;
  }
}

export const currentPage = writable('terminals');

// Terminal font size (zoom): Ctrl + / Ctrl - / Ctrl 0, Ctrl + wheel, or
// Settings. Applies to every terminal, remembered per machine.
export const TERMINAL_FONT_MIN = 8;
export const TERMINAL_FONT_MAX = 28;
export const TERMINAL_FONT_DEFAULT = 13;
const FONT_KEY = 'mimir-terminal-font-size';

export function clampFontSize(n) {
  const v = Math.round(Number(n));
  if (!Number.isFinite(v)) return TERMINAL_FONT_DEFAULT;
  return Math.min(TERMINAL_FONT_MAX, Math.max(TERMINAL_FONT_MIN, v));
}

function initialFontSize() {
  try {
    const saved = localStorage.getItem(FONT_KEY);
    return saved ? clampFontSize(saved) : TERMINAL_FONT_DEFAULT;
  } catch {
    return TERMINAL_FONT_DEFAULT;
  }
}

export const terminalFontSize = writable(initialFontSize());
terminalFontSize.subscribe((size) => {
  try { localStorage.setItem(FONT_KEY, String(size)); } catch { /* ignore */ }
});

export function setTerminalFontSize(n) {
  terminalFontSize.set(clampFontSize(n));
}

export function zoomTerminalFont(delta) {
  terminalFontSize.update((size) => clampFontSize(size + delta));
}

export function resetTerminalFontSize() {
  terminalFontSize.set(TERMINAL_FONT_DEFAULT);
}
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
