import { derived, writable } from 'svelte/store';
import { collectLeafIds } from '../terminals/layoutTree.js';
import { defaultTerminalTypeOptions, isWindowsPlatform, readDefaultTerminalType } from '../terminals/terminalTypes.js';

export const terminals = writable([]);
export const activeTerminalId = writable(null);
export const layoutTree = writable(null);

// The "+ New" picker: offered shells and the remembered default type.
export const availableTerminalTypes = writable(defaultTerminalTypeOptions());
export const selectedTerminalType = writable(readDefaultTerminalType(defaultTerminalTypeOptions()));

// ID of the terminal currently being dragged, or null. Shared so every pane can
// show drop zones while a drag is in progress (each pane is a separate SplitPane
// instance, so a component-local flag would not reach the other panes).
export const draggingTerminalId = writable(null);

export const terminalMap = derived(terminals, ($terminals) => new Map($terminals.map((terminal) => [terminal.id, terminal])));

export const visibleTerminalCount = derived(layoutTree, ($layoutTree) => ($layoutTree ? collectLeafIds($layoutTree).length : 0));

// The shell type discovery runs against: the active terminal's, else the
// picker's default, else the platform's.
export const discoveryTerminalType = derived(
  [terminals, activeTerminalId, selectedTerminalType],
  ([$terminals, $activeTerminalId, $selectedTerminalType]) => {
    const activeTerminal = $terminals.find((terminal) => terminal.id === $activeTerminalId);
    return activeTerminal?.type || $selectedTerminalType || (isWindowsPlatform ? 'powershell' : 'bash');
  },
);
