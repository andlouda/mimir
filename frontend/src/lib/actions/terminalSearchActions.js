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

export function terminalSearchNext(id) {
  const term = get(terminals).find((terminal) => terminal.id === id);
  if (term?.searchQuery) {
    term.searchAddon.findNext(term.searchQuery);
  }
}

export function terminalSearchPrev(id) {
  const term = get(terminals).find((terminal) => terminal.id === id);
  if (term?.searchQuery) {
    term.searchAddon.findPrevious(term.searchQuery);
  }
}

export function updateTerminalSearchQuery(id, query) {
  terminals.update((items) => items.map((terminal) => {
    if (terminal.id !== id) return terminal;
    return { ...terminal, searchQuery: query };
  }));

  const term = get(terminals).find((terminal) => terminal.id === id);
  if (!term) return;
  if (query) {
    term.searchAddon.findNext(query);
  } else {
    term.searchAddon.clearDecorations();
  }
}

export function dismissRestoreSummary(id) {
  terminals.update((items) => items.map((terminal) => {
    if (terminal.id !== id) return terminal;
    return { ...terminal, restoreDismissed: true };
  }));
}
