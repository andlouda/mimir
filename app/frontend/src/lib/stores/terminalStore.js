import { derived, writable } from 'svelte/store';
import { collectLeafIds } from '../terminals/layoutTree.js';

export const terminals = writable([]);
export const activeTerminalId = writable(null);
export const layoutTree = writable(null);

export const terminalMap = derived(terminals, ($terminals) => new Map($terminals.map((terminal) => [terminal.id, terminal])));

export const visibleTerminalCount = derived(layoutTree, ($layoutTree) => ($layoutTree ? collectLeafIds($layoutTree).length : 0));
