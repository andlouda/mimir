import { afterEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { createKeydownHandler } from './keyboardShortcuts.js';
import { notesPanelOpen } from '../stores/uiStore.js';
import { showTemplatePicker, showWorkflowPicker } from '../stores/templateStore.js';
import { terminals } from '../stores/terminalStore.js';

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
  vi.restoreAllMocks();
});

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
});
