import { get } from 'svelte/store';
import { notesPanelOpen } from '../stores/uiStore.js';
import { showTemplatePicker, showWorkflowPicker } from '../stores/templateStore.js';
import { activeTerminalId, layoutTree, terminals } from '../stores/terminalStore.js';
import { collectLeafIds } from '../terminals/layoutTree.js';

// Pane navigation: Ctrl+Shift+Left/Right or Ctrl(+Shift)+Tab cycles through
// the visible panes in layout order, Ctrl+Shift+1…9 jumps to the n-th pane,
// Ctrl+Shift+T opens a new terminal of the default type. Digits use
// event.code so the binding works on every keyboard layout.
const DIGIT_CODE = /^Digit([1-9])$/;

/**
 * True when the event is one of Mimir's global shortcuts. Terminals use
 * this as their custom key handler so the keystroke is not also sent to the
 * shell (xterm would otherwise forward e.g. Ctrl+Shift+Right as an escape
 * sequence before the window handler runs).
 */
export function isGlobalShortcut(event) {
  if (!event?.ctrlKey) return false;
  if (event.key === 'Tab') return true;
  if (!event.shiftKey) return false;
  if (['ArrowLeft', 'ArrowRight'].includes(event.key)) return true;
  if (DIGIT_CODE.test(event.code || '')) return true;
  return ['N', 'n', 'F', 'f', 'P', 'p', 'W', 'w', 'T', 't'].includes(event.key);
}

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

export function jumpToTerminal(index) {
  const ids = visiblePaneIds();
  if (index >= 0 && index < ids.length) focusTerminal(ids[index]);
}

export function createKeydownHandler({
  handleResize,
  toggleTerminalSearch,
  toggleWorkflowPicker,
  closeTerminalSearch,
  addTerminal,
  schedule = setTimeout,
} = {}) {
  return function handleGlobalKeydown(event) {
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

    const digit = event.ctrlKey && event.shiftKey ? DIGIT_CODE.exec(event.code || '') : null;
    if (digit) {
      event.preventDefault();
      jumpToTerminal(Number(digit[1]) - 1);
      return;
    }

    if (event.ctrlKey && event.shiftKey && (event.key === 'T' || event.key === 't')) {
      event.preventDefault();
      if (typeof addTerminal === 'function') addTerminal();
      return;
    }

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
