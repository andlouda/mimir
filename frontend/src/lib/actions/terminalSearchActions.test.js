import { afterEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  closeTerminalSearch,
  dismissRestoreSummary,
  terminalSearchNext,
  terminalSearchPrev,
  toggleTerminalSearch,
  updateTerminalSearchQuery,
} from './terminalSearchActions.js';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';

function terminal(overrides = {}) {
  return {
    id: 1,
    searchVisible: false,
    searchQuery: '',
    restoreDismissed: false,
    searchAddon: {
      clearDecorations: vi.fn(),
      findNext: vi.fn(),
      findPrevious: vi.fn(),
    },
    terminal: {
      focus: vi.fn(),
    },
    ...overrides,
  };
}

afterEach(() => {
  activeTerminalId.set(null);
  terminals.set([]);
  vi.restoreAllMocks();
});

describe('terminal search actions', () => {
  test('toggles search for the active terminal and clears when closing', () => {
    const term = terminal({ searchVisible: true, searchQuery: 'logs' });
    terminals.set([term]);
    activeTerminalId.set(1);

    toggleTerminalSearch();

    const updated = get(terminals)[0];
    expect(updated.searchVisible).toBe(false);
    expect(updated.searchQuery).toBe('');
    expect(term.searchAddon.clearDecorations).toHaveBeenCalledOnce();
    expect(term.terminal.focus).toHaveBeenCalledOnce();
  });

  test('updates query and navigates matches', () => {
    const term = terminal();
    terminals.set([term]);

    updateTerminalSearchQuery(1, 'error');
    terminalSearchNext(1);
    terminalSearchPrev(1);

    expect(get(terminals)[0].searchQuery).toBe('error');
    expect(term.searchAddon.findNext).toHaveBeenCalledWith('error');
    expect(term.searchAddon.findPrevious).toHaveBeenCalledWith('error');
  });

  test('closes search and dismisses restore summary', () => {
    const term = terminal({ searchVisible: true, searchQuery: 'warn' });
    terminals.set([term]);

    closeTerminalSearch(1);
    dismissRestoreSummary(1);

    expect(get(terminals)[0]).toMatchObject({
      searchVisible: false,
      searchQuery: '',
      restoreDismissed: true,
    });
    expect(term.searchAddon.clearDecorations).toHaveBeenCalledOnce();
  });
});
