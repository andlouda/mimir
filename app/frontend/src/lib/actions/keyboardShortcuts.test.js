import { afterEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { createKeydownHandler, isGlobalShortcut } from './keyboardShortcuts.js';
import { notesPanelOpen } from '../stores/uiStore.js';
import { showTemplatePicker, showWorkflowPicker } from '../stores/templateStore.js';
import { activeTerminalId, layoutTree, terminals } from '../stores/terminalStore.js';

function keyEvent(overrides) {
  return {
    ctrlKey: false,
    shiftKey: false,
    key: '',
    preventDefault: vi.fn(),
    ...overrides,
  };
}

afterEach(() => {
  notesPanelOpen.set(false);
  showTemplatePicker.set(false);
  showWorkflowPicker.set(false);
  terminals.set([]);
  layoutTree.set(null);
  activeTerminalId.set(null);
  vi.restoreAllMocks();
});

function threePanes() {
  const focus = vi.fn();
  terminals.set([1, 2, 3].map((id) => ({ id, terminal: { focus } })));
  layoutTree.set({
    type: 'split', direction: 'horizontal', ratio: 0.5,
    children: [{ type: 'leaf', terminalId: 1 }, { type: 'split', direction: 'vertical', ratio: 0.5, children: [{ type: 'leaf', terminalId: 2 }, { type: 'leaf', terminalId: 3 }] }],
  });
  return focus;
}

describe('keyboard shortcuts', () => {
  test('toggles notes panel and schedules resize', () => {
    const handleResize = vi.fn();
    const schedule = vi.fn();
    const handler = createKeydownHandler({ handleResize, schedule });
    const event = keyEvent({ ctrlKey: true, shiftKey: true, key: 'N' });

    handler(event);

    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(get(notesPanelOpen)).toBe(true);
    expect(schedule).toHaveBeenCalledWith(handleResize, 50);
  });

  test('dispatches search, template, and workflow shortcuts', () => {
    const toggleTerminalSearch = vi.fn();
    const toggleWorkflowPicker = vi.fn();
    const handler = createKeydownHandler({ toggleTerminalSearch, toggleWorkflowPicker });

    handler(keyEvent({ ctrlKey: true, shiftKey: true, key: 'F' }));
    handler(keyEvent({ ctrlKey: true, shiftKey: true, key: 'P' }));
    handler(keyEvent({ ctrlKey: true, shiftKey: true, key: 'w' }));

    expect(toggleTerminalSearch).toHaveBeenCalledOnce();
    expect(get(showTemplatePicker)).toBe(true);
    expect(toggleWorkflowPicker).toHaveBeenCalledOnce();
  });

  test('escape closes pickers before terminal searches', () => {
    const closeTerminalSearch = vi.fn();
    const handler = createKeydownHandler({ closeTerminalSearch });
    showTemplatePicker.set(true);
    showWorkflowPicker.set(true);
    terminals.set([{ id: 7, searchVisible: true }]);

    handler(keyEvent({ key: 'Escape' }));
    expect(get(showTemplatePicker)).toBe(false);
    expect(get(showWorkflowPicker)).toBe(true);
    expect(closeTerminalSearch).not.toHaveBeenCalled();

    handler(keyEvent({ key: 'Escape' }));
    expect(get(showWorkflowPicker)).toBe(false);
    expect(closeTerminalSearch).not.toHaveBeenCalled();

    handler(keyEvent({ key: 'Escape' }));
    expect(closeTerminalSearch).toHaveBeenCalledWith(7);
  });

  test('cycles panes with Ctrl+Tab / Ctrl+Shift+Arrows and focuses the terminal', () => {
    const focus = threePanes();
    const handler = createKeydownHandler();
    activeTerminalId.set(1);

    handler(keyEvent({ ctrlKey: true, key: 'Tab' }));
    expect(get(activeTerminalId)).toBe(2);
    handler(keyEvent({ ctrlKey: true, shiftKey: true, key: 'Tab' }));
    expect(get(activeTerminalId)).toBe(1);
    handler(keyEvent({ ctrlKey: true, shiftKey: true, key: 'ArrowLeft' }));
    expect(get(activeTerminalId)).toBe(3); // wraps around
    handler(keyEvent({ ctrlKey: true, shiftKey: true, key: 'ArrowRight' }));
    expect(get(activeTerminalId)).toBe(1);
    expect(focus).toHaveBeenCalledTimes(4);
  });

  test('jumps to the n-th pane with Ctrl+Shift+digit and opens a new terminal with Ctrl+Shift+T', () => {
    threePanes();
    const addTerminal = vi.fn();
    const handler = createKeydownHandler({ addTerminal });

    handler(keyEvent({ ctrlKey: true, shiftKey: true, key: '!', code: 'Digit3' }));
    expect(get(activeTerminalId)).toBe(3);
    handler(keyEvent({ ctrlKey: true, shiftKey: true, key: '(', code: 'Digit9' }));
    expect(get(activeTerminalId)).toBe(3); // out of range: unchanged

    const event = keyEvent({ ctrlKey: true, shiftKey: true, key: 'T' });
    handler(event);
    expect(addTerminal).toHaveBeenCalledOnce();
    expect(event.preventDefault).toHaveBeenCalled();
  });

  test('isGlobalShortcut only claims Mimir combos', () => {
    expect(isGlobalShortcut(keyEvent({ ctrlKey: true, key: 'Tab' }))).toBe(true);
    expect(isGlobalShortcut(keyEvent({ ctrlKey: true, shiftKey: true, key: 'ArrowRight' }))).toBe(true);
    expect(isGlobalShortcut(keyEvent({ ctrlKey: true, shiftKey: true, key: '!', code: 'Digit1' }))).toBe(true);
    expect(isGlobalShortcut(keyEvent({ ctrlKey: true, shiftKey: true, key: 'F' }))).toBe(true);
    expect(isGlobalShortcut(keyEvent({ ctrlKey: true, key: 'c' }))).toBe(false); // Ctrl+C stays with the shell
    expect(isGlobalShortcut(keyEvent({ ctrlKey: true, shiftKey: true, key: 'C' }))).toBe(false); // copy stays with xterm
    expect(isGlobalShortcut(keyEvent({ shiftKey: true, key: 'ArrowRight' }))).toBe(false);
  });
});
