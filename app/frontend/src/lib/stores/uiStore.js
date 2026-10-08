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
// Slow shared blink for status dots. An infinite CSS animation, even on a
// 6 px dot, keeps WebKitGTK's compositor redrawing the whole window at
// display rate for as long as any agent works; a class toggled 1.4 times
// a second costs a small repaint instead. Ticks only while subscribed.
export const uiBlink = (() => {
  let timer = null;
  let subscribers = 0;
  const { subscribe, update, set } = writable(false);
  return {
    subscribe(run, invalidate) {
      if (subscribers++ === 0) timer = setInterval(() => update((v) => !v), 700);
      const unsub = subscribe(run, invalidate);
      return () => {
        unsub();
        if (--subscribers === 0) { clearInterval(timer); timer = null; set(false); }
      };
    },
  };
})();

// CPU and memory overlay in each pane's top-right corner; on by default.
const PANE_RESOURCES_KEY = 'mimir-pane-resources';
function initialPaneResourceOverlay() {
  try { return localStorage.getItem(PANE_RESOURCES_KEY) !== 'off'; } catch { return true; }
}
export const paneResourceOverlay = writable(initialPaneResourceOverlay());
paneResourceOverlay.subscribe((on) => {
  try { localStorage.setItem(PANE_RESOURCES_KEY, on ? 'on' : 'off'); } catch { /* ignore */ }
});

// How the overlay shows CPU: 'cores' = CPUs in use ("3.4 CPU"), 'machine' =
// percent of all cores (like the system monitor), 'core' = percent of one
// core (like htop).
const PANE_RESOURCE_SCALE_KEY = 'mimir-pane-resource-scale';
export const PANE_RESOURCE_SCALES = ['machine', 'core', 'cores'];
function initialPaneResourceScale() {
  try {
    const v = localStorage.getItem(PANE_RESOURCE_SCALE_KEY);
    return PANE_RESOURCE_SCALES.includes(v) ? v : 'cores';
  } catch { return 'cores'; }
}
export const paneResourceScale = writable(initialPaneResourceScale());
paneResourceScale.subscribe((v) => {
  try { localStorage.setItem(PANE_RESOURCE_SCALE_KEY, v); } catch { /* ignore */ }
});

// Draw inactive panes in batches (see terminals/outputThrottle.js); on by default.
const INACTIVE_THROTTLE_KEY = 'mimir-inactive-pane-throttle';
function initialInactiveThrottle() {
  try { return localStorage.getItem(INACTIVE_THROTTLE_KEY) !== 'off'; } catch { return true; }
}
export const inactivePaneThrottle = writable(initialInactiveThrottle());
inactivePaneThrottle.subscribe((on) => {
  try { localStorage.setItem(INACTIVE_THROTTLE_KEY, on ? 'on' : 'off'); } catch { /* ignore */ }
});
// How often an inactive pane's backlog is drawn, in milliseconds. One
// second cuts the redraw work of idle agent panes to a tenth of 100 ms
// while the pane still looks alive; 30 s is for many panes on a weak
// machine. Switching to a pane always draws its backlog at once.
const INACTIVE_INTERVAL_KEY = 'mimir-inactive-pane-interval';
export const INACTIVE_PANE_INTERVALS = [100, 500, 1000, 5000, 30000];
function initialInactiveInterval() {
  try {
    const v = Number(localStorage.getItem(INACTIVE_INTERVAL_KEY));
    return INACTIVE_PANE_INTERVALS.includes(v) ? v : 1000;
  } catch { return 1000; }
}
export const inactivePaneInterval = writable(initialInactiveInterval());
inactivePaneInterval.subscribe((v) => {
  try { localStorage.setItem(INACTIVE_INTERVAL_KEY, String(v)); } catch { /* ignore */ }
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
