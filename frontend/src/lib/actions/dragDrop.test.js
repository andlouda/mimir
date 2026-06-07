import { afterEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { createDragDropHandlers } from './dragDrop.js';
import { layoutTree } from '../stores/terminalStore.js';

function classList(initial = []) {
  const classes = new Set(initial);
  return {
    add: (...names) => names.forEach((name) => classes.add(name)),
    remove: (...names) => names.forEach((name) => classes.delete(name)),
    contains: (name) => classes.has(name),
    has: (name) => classes.has(name),
  };
}

afterEach(() => {
  layoutTree.set(null);
  vi.restoreAllMocks();
});

describe('drag drop handlers', () => {
  test('swaps terminal leaves on drop', () => {
    layoutTree.set({
      type: 'split',
      direction: 'horizontal',
      ratio: 0.5,
      children: [
        { type: 'leaf', terminalId: 1 },
        { type: 'leaf', terminalId: 2 },
      ],
    });
    const reinitializeTerminals = vi.fn();
    const currentTargetClasses = classList(['drag-over-top']);
    const { handleDrop } = createDragDropHandlers({ reinitializeTerminals });

    handleDrop({
      preventDefault: vi.fn(),
      currentTarget: { classList: currentTargetClasses },
      dataTransfer: { getData: () => '1' },
    }, 2);

    expect(get(layoutTree).children.map((child) => child.terminalId)).toEqual([2, 1]);
    expect(reinitializeTerminals).toHaveBeenCalledOnce();
    expect(currentTargetClasses.has('drag-over-top')).toBe(false);
  });

  test('marks top or bottom drop target while dragging over headers', () => {
    const classes = classList(['terminal-header']);
    const { handleDragStart, handleDragOver } = createDragDropHandlers();

    handleDragStart({
      dataTransfer: { setData: vi.fn(), effectAllowed: '' },
      currentTarget: { classList: classList() },
    }, 1);
    handleDragOver({
      preventDefault: vi.fn(),
      currentTarget: {
        classList: classes,
        getBoundingClientRect: () => ({ top: 0, height: 100 }),
      },
      clientY: 75,
    }, 2);

    expect(classes.has('drag-over-bottom')).toBe(true);
    expect(classes.has('drag-over-top')).toBe(false);
  });
});
