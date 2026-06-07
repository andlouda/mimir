import { afterEach, describe, expect, test } from 'vitest';
import { get } from 'svelte/store';
import { activeTerminalId, layoutTree, terminalMap, terminals, visibleTerminalCount } from './terminalStore.js';

afterEach(() => {
  terminals.set([]);
  activeTerminalId.set(null);
  layoutTree.set(null);
});

describe('terminal store', () => {
  test('derives a terminal lookup map by id', () => {
    terminals.set([
      { id: 1, name: 'Bash' },
      { id: 2, name: 'SSH' },
    ]);

    expect(get(terminalMap).get(1).name).toBe('Bash');
    expect(get(terminalMap).get(2).name).toBe('SSH');
  });

  test('derives visible terminal count from the layout tree', () => {
    layoutTree.set({
      type: 'split',
      direction: 'horizontal',
      ratio: 0.5,
      children: [
        { type: 'leaf', terminalId: 1 },
        { type: 'leaf', terminalId: 2 },
      ],
    });

    expect(get(visibleTerminalCount)).toBe(2);
  });
});
