import { writable } from 'svelte/store';

function initialNotesPanelWidth() {
  try {
    return parseInt(localStorage.getItem('mimir-notes-width') || '380');
  } catch {
    return 380;
  }
}

export const currentPage = writable('terminals');
// Shell prompt for new terminals: 'mimir' (short "dir $ " prompt) or
// 'shell' (keep the user's own prompt). Loaded from the backend at start.
export const promptMode = writable('mimir');
export async function loadPromptMode() {
  const getter = globalThis.window?.['go']?.['main']?.['App']?.['GetPromptMode'];
  if (typeof getter !== 'function') return;
  try { promptMode.set((await getter()) === 'shell' ? 'shell' : 'mimir'); } catch { /* keep default */ }
}

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

// Terminal renderer: 'auto' tries WebGL (fast, draws box-drawing and block
// glyphs itself) and falls back to the DOM renderer; 'dom' forces the DOM
// renderer (compatibility). Remembered per machine.
const RENDERER_KEY = 'mimir-terminal-renderer';
function initialRenderer() {
  try { return localStorage.getItem(RENDERER_KEY) === 'dom' ? 'dom' : 'auto'; } catch { return 'auto'; }
}
export const terminalRenderer = writable(initialRenderer());
terminalRenderer.subscribe((mode) => {
  try { localStorage.setItem(RENDERER_KEY, mode); } catch { /* ignore */ }
});
// Re-fill a tmux pane's scrollback after it was widened (see
// terminals/tmuxScrollback.js); on by default.
const SCROLLBACK_REFILL_KEY = 'mimir-tmux-scrollback-refill';
function initialScrollbackRefill() {
  try { return localStorage.getItem(SCROLLBACK_REFILL_KEY) !== 'off'; } catch { return true; }
}
export const tmuxScrollbackRefill = writable(initialScrollbackRefill());
tmuxScrollbackRefill.subscribe((on) => {
  try { localStorage.setItem(SCROLLBACK_REFILL_KEY, on ? 'on' : 'off'); } catch { /* ignore */ }
});
// Restart Claude Code with --resume after its pane was widened, so the
// output is printed again at the new width (see agents/claudeRerender.js).
const CLAUDE_RERENDER_KEY = 'mimir-claude-rerender';
function initialClaudeRerender() {
  try { return localStorage.getItem(CLAUDE_RERENDER_KEY) !== 'off'; } catch { return true; }
}
export const claudeRerenderOnWiden = writable(initialClaudeRerender());
claudeRerenderOnWiden.subscribe((on) => {
  try { localStorage.setItem(CLAUDE_RERENDER_KEY, on ? 'on' : 'off'); } catch { /* ignore */ }
});
// tmux integration for new terminals: 'invisible' | 'classic' | 'off'
// (loaded from the backend at start-up; see terminal/tmux_options.go).
export const tmuxIntegrationMode = writable('invisible');
export const errorMessage = writable('');
export const showAIMenu = writable(false);
export const notesPanelOpen = writable(false);
// Filename the notes panel should open as soon as it is shown (set by
// "open in notes" after an import; cleared by the panel).
export const notesOpenRequest = writable('');
export const notesPanelWidth = writable(initialNotesPanelWidth());
export const showFolderManager = writable(false);
export const historyTrackingEnabled = writable(false);
export const historyConsentDismissed = writable(false);
export const transcriptViewerState = writable(null);
// When set to { terminalId, terminalType, label } the secure .env viewer modal opens.
export const dotEnvViewerState = writable(null);
