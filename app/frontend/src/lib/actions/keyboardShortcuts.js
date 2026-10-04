import { get } from 'svelte/store';
import { notesPanelOpen, resetTerminalFontSize, zoomTerminalFont } from '../stores/uiStore.js';
import { showTemplatePicker, showWorkflowPicker } from '../stores/templateStore.js';
import { activeTerminalId, layoutTree, terminals } from '../stores/terminalStore.js';
import { customFolders } from '../stores/sessionStore.js';
import { collectLeafIds } from '../terminals/layoutTree.js';
import { sidebarOrderIds } from '../terminals/sidebarGroups.js';

// Pane navigation: Ctrl+Shift+Left/Right or Ctrl(+Shift)+Tab cycles through
// the visible panes in layout order; Ctrl+Shift+1…9 jumps to the n-th
// terminal as listed in the sidebar (minimized ones are restored first);
// Ctrl+Shift+M minimizes the active pane, Ctrl+Shift+O (or U) brings the
// most recently minimized one back; Ctrl+Shift+T opens a new terminal of the
// default type. Digits use event.code so the binding works on every
// keyboard layout.
const DIGIT_CODE = /^Digit([1-9])$/;

/**
 * True when the event is one of Mimir's global shortcuts. Terminals use
 * this as their custom key handler so the keystroke is not also sent to the
 * shell (xterm would otherwise forward e.g. Ctrl+Shift+Right as an escape
 * sequence before the window handler runs).
 */
const ZOOM_KEYS = ['+', '=', '-', '0'];

// Physical-key letter for shortcuts: "KeyN" → "N" whatever CapsLock or the
// layout (Cyrillic, Greek, …) makes of event.key; falls back to the key.
export function letterOf(event) {
  const m = /^Key([A-Z])$/.exec(event?.code || '');
  if (m) return m[1];
  const k = String(event?.key || '');
  return k.length === 1 ? k.toUpperCase() : k;
}

// ---- overlay stack ---------------------------------------------------------
// Escape closes the topmost Mimir overlay. Components with their own overlay
// state (the pane context menu) register here; pickers are stores; terminal
// search bars live on the terminal objects. The xterm key handler asks this
// first, because xterm stops propagation of every key it handles and the
// window listener would never see Escape while a terminal has focus.
const overlays = []; // { isOpen, close } — last registered = topmost
export function registerOverlay(isOpen, close) {
  const entry = { isOpen, close };
  overlays.push(entry);
  return () => {
    const i = overlays.indexOf(entry);
    if (i >= 0) overlays.splice(i, 1);
  };
}

export function closeTopOverlay() {
  for (let i = overlays.length - 1; i >= 0; i--) {
    if (overlays[i].isOpen()) { overlays[i].close(); return true; }
  }
  if (get(showTemplatePicker)) { showTemplatePicker.set(false); return true; }
  if (get(showWorkflowPicker)) { showWorkflowPicker.set(false); return true; }
  const visibleSearches = get(terminals).filter((terminal) => terminal.searchVisible);
  if (visibleSearches.length && overlayHooks.closeTerminalSearch) {
    for (const terminal of visibleSearches) overlayHooks.closeTerminalSearch(terminal.id);
    return true;
  }
  return false;
}

export function hasOpenOverlay() {
  return overlays.some((o) => o.isOpen()) || get(showTemplatePicker) || get(showWorkflowPicker)
    || get(terminals).some((terminal) => terminal.searchVisible);
}

const overlayHooks = { closeTerminalSearch: null };

export function isZoomShortcut(event) {
  return !!event?.ctrlKey && !event.altKey && !event.metaKey && ZOOM_KEYS.includes(event.key);
}

export function isGlobalShortcut(event) {
  if (!event?.ctrlKey) return false;
  if (event.key === 'Tab') return true;
  if (isZoomShortcut(event)) return true;
  if (!event.shiftKey) return false;
  if (['ArrowLeft', 'ArrowRight'].includes(event.key)) return true;
  if (DIGIT_CODE.test(event.code || '')) return true;
  return ['N', 'F', 'P', 'W', 'T', 'M', 'U', 'O'].includes(letterOf(event));
}

// Terminals minimized through the shortcut, most recent last, so Ctrl+Shift+U
// restores them in reverse order.
const minimizedStack = [];

function visiblePaneIds() {
  const tree = get(layoutTree);
  return tree ? collectLeafIds(tree) : [];
}

export function focusTerminal(id) {
  if (id == null) return;
  activeTerminalId.set(id);
  const term = get(terminals).find((t) => t.id === id);
  try {
    term?.terminal?.focus?.();
  } catch (error) {
    console.error(`Failed to focus terminal ${id}:`, error);
  }
}

export function cycleTerminal(step) {
  const ids = visiblePaneIds();
  if (ids.length === 0) return;
  const current = ids.indexOf(get(activeTerminalId));
  const next = current < 0 ? (step > 0 ? 0 : ids.length - 1) : (current + step + ids.length) % ids.length;
  focusTerminal(ids[next]);
}

/** Jumps to the n-th terminal in sidebar order, restoring it if minimized. */
export async function jumpToTerminal(index, toggleMinimize) {
  const ids = sidebarOrderIds(get(terminals), get(customFolders));
  if (index < 0 || index >= ids.length) return;
  const id = ids[index];
  const term = get(terminals).find((t) => t.id === id);
  if (term?.minimized) {
    if (typeof toggleMinimize !== 'function') return;
    await toggleMinimize(id);
  }
  focusTerminal(id);
}

/** Minimizes the active pane and moves focus to the next visible one. */
export async function minimizeActiveTerminal(toggleMinimize) {
  const id = get(activeTerminalId);
  if (id == null || typeof toggleMinimize !== 'function') return;
  const term = get(terminals).find((t) => t.id === id);
  if (!term || term.minimized) return;
  const visible = visiblePaneIds();
  const position = visible.indexOf(id);
  minimizedStack.push(id);
  await toggleMinimize(id);
  const remaining = visiblePaneIds();
  if (remaining.length === 0) return;
  focusTerminal(remaining[Math.min(Math.max(position, 0), remaining.length - 1)]);
}

/** Restores the most recently minimized terminal. */
export async function restoreMinimizedTerminal(toggleMinimize) {
  if (typeof toggleMinimize !== 'function') return;
  const list = get(terminals);
  let id = null;
  while (minimizedStack.length) {
    const candidate = minimizedStack.pop();
    if (list.find((t) => t.id === candidate && t.minimized)) { id = candidate; break; }
  }
  if (id == null) {
    const minimized = list.filter((t) => t.minimized);
    if (minimized.length === 0) return;
    id = minimized[minimized.length - 1].id;
  }
  await toggleMinimize(id);
  focusTerminal(id);
}

export function createKeydownHandler({
  handleResize,
  toggleTerminalSearch,
  toggleWorkflowPicker,
  closeTerminalSearch,
  addTerminal,
  toggleMinimize,
  schedule = setTimeout,
} = {}) {
  overlayHooks.closeTerminalSearch = closeTerminalSearch || null;
  return function handleGlobalKeydown(event) {
    // IME composition and auto-repeat: a held Ctrl+Shift+T must not open a
    // terminal per repeat; cycling (Tab / arrows) may repeat.
    if (event.isComposing || event.keyCode === 229) return;
    const cycling = event.ctrlKey && (event.key === 'Tab' || event.key === 'ArrowLeft' || event.key === 'ArrowRight');
    if (event.repeat && !cycling) return;

    if (event.ctrlKey && event.key === 'Tab') {
      event.preventDefault();
      cycleTerminal(event.shiftKey ? -1 : 1);
      return;
    }

    if (event.ctrlKey && event.shiftKey && (event.key === 'ArrowRight' || event.key === 'ArrowLeft')) {
      event.preventDefault();
      cycleTerminal(event.key === 'ArrowRight' ? 1 : -1);
      return;
    }

    // Zoom: Ctrl + / Ctrl = grow, Ctrl - shrink, Ctrl 0 reset (all terminals).
    if (isZoomShortcut(event)) {
      event.preventDefault();
      if (event.key === '0') resetTerminalFontSize();
      else zoomTerminalFont(event.key === '-' ? -1 : 1);
      return;
    }

    const digit = event.ctrlKey && event.shiftKey ? DIGIT_CODE.exec(event.code || '') : null;
    if (digit) {
      event.preventDefault();
      jumpToTerminal(Number(digit[1]) - 1, toggleMinimize);
      return;
    }

    if (event.ctrlKey && event.shiftKey && letterOf(event) === 'M') {
      event.preventDefault();
      minimizeActiveTerminal(toggleMinimize);
      return;
    }

    // Ctrl+Shift+O is the primary restore key: Ctrl+Shift+U is GTK's
    // Unicode-input chord on Linux and never reaches the page there. U stays
    // as an alias for platforms where it works.
    if (event.ctrlKey && event.shiftKey && ['O', 'U'].includes(letterOf(event))) {
      event.preventDefault();
      restoreMinimizedTerminal(toggleMinimize);
      return;
    }

    if (event.ctrlKey && event.shiftKey && letterOf(event) === 'T') {
      event.preventDefault();
      if (typeof addTerminal === 'function') addTerminal();
      return;
    }

    if (event.ctrlKey && event.shiftKey && letterOf(event) === 'N') {
      event.preventDefault();
      notesPanelOpen.update((open) => !open);
      schedule(handleResize, 50);
      return;
    }

    if (event.ctrlKey && event.shiftKey && letterOf(event) === 'F') {
      event.preventDefault();
      toggleTerminalSearch();
      return;
    }

    if (event.ctrlKey && event.shiftKey && letterOf(event) === 'P') {
      event.preventDefault();
      showTemplatePicker.update((open) => !open);
      return;
    }

    if (event.ctrlKey && event.shiftKey && letterOf(event) === 'W') {
      event.preventDefault();
      toggleWorkflowPicker();
      return;
    }

    if (event.key === 'Escape') {
      closeTopOverlay();
    }
  };
}
