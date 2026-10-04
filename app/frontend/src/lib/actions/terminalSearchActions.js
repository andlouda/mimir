import { get } from 'svelte/store';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';

export function toggleTerminalSearch() {
  const selectedId = get(activeTerminalId);
  if (!selectedId) return;

  terminals.update((items) => items.map((terminal) => {
    if (terminal.id !== selectedId) return terminal;
    return {
      ...terminal,
      searchVisible: !terminal.searchVisible,
      searchQuery: terminal.searchVisible ? '' : terminal.searchQuery,
    };
  }));

  const term = get(terminals).find((terminal) => terminal.id === selectedId);
  if (term && !term.searchVisible) {
    term.searchAddon.clearDecorations();
    term.terminal.focus();
  }
}

export function closeTerminalSearch(id) {
  terminals.update((items) => items.map((terminal) => {
    if (terminal.id !== id) return terminal;
    return { ...terminal, searchVisible: false, searchQuery: '' };
  }));

  const term = get(terminals).find((terminal) => terminal.id === id);
  if (term) {
    term.searchAddon.clearDecorations();
    term.terminal.focus();
  }
}

// Decorations make every match visible (and in the overview ruler), not
// just the one selected; the active match is brighter.
export const SEARCH_OPTIONS = {
  decorations: {
    matchBackground: '#5a4a1a',
    matchBorder: '#e3b341',
    matchOverviewRuler: '#e3b341',
    activeMatchBackground: '#e3b341',
    activeMatchBorder: '#ffffff',
    activeMatchColorOverviewRuler: '#ffffff',
  },
};

export function terminalSearchNext(id) {
  const term = get(terminals).find((terminal) => terminal.id === id);
  if (term?.searchQuery) {
    term.searchAddon.findNext(term.searchQuery, SEARCH_OPTIONS);
  }
}

export function terminalSearchPrev(id) {
  const term = get(terminals).find((terminal) => terminal.id === id);
  if (term?.searchQuery) {
    term.searchAddon.findPrevious(term.searchQuery, SEARCH_OPTIONS);
  }
}

/** Called by the search addon's onDidChangeResults; -1 means no match. */
export function updateTerminalSearchResult(id, resultIndex, resultCount) {
  terminals.update((items) => items.map((terminal) => {
    if (terminal.id !== id) return terminal;
    return { ...terminal, searchResult: { index: resultIndex, count: resultCount } };
  }));
}

export function updateTerminalSearchQuery(id, query) {
  terminals.update((items) => items.map((terminal) => {
    if (terminal.id !== id) return terminal;
    return { ...terminal, searchQuery: query };
  }));

  const term = get(terminals).find((terminal) => terminal.id === id);
  if (!term) return;
  if (query) {
    term.searchAddon.findNext(query, SEARCH_OPTIONS);
  } else {
    term.searchAddon.clearDecorations();
    updateTerminalSearchResult(id, -1, 0);
  }
}

export function dismissRestoreSummary(id) {
  terminals.update((items) => items.map((terminal) => {
    if (terminal.id !== id) return terminal;
    return { ...terminal, restoreDismissed: true };
  }));
}
