import { derived, writable } from 'svelte/store';
import { collectLeafIds } from '../terminals/layoutTree.js';

export const terminals = writable([]);
export const activeTerminalId = writable(null);
export const layoutTree = writable(null);

// ID of the terminal currently being dragged, or null. Shared so every pane can
// show drop zones while a drag is in progress (each pane is a separate SplitPane
// instance, so a component-local flag would not reach the other panes).
export const draggingTerminalId = writable(null);

export const terminalMap = derived(terminals, ($terminals) => new Map($terminals.map((terminal) => [terminal.id, terminal])));

export const visibleTerminalCount = derived(layoutTree, ($layoutTree) => ($layoutTree ? collectLeafIds($layoutTree).length : 0));
