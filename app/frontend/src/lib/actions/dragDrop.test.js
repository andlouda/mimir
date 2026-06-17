import { afterEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { createDragDropHandlers } from './dragDrop.js';
import { layoutTree, draggingTerminalId } from '../stores/terminalStore.js';

function classList(initial = []) {
  const classes = new Set(initial);
  return {
    add: (...names) => names.forEach((name) => classes.add(name)),
    remove: (...names) => names.forEach((name) => classes.delete(name)),
    contains: (name) => classes.has(name),
    has: (name) => classes.has(name),
  };
}

function horizontalTree() {
  return {
    type: 'split',
    direction: 'horizontal',
    ratio: 0.5,
    children: [
      { type: 'leaf', terminalId: 1 },
      { type: 'leaf', terminalId: 2 },
    ],
  };
}

afterEach(() => {
  layoutTree.set(null);
  draggingTerminalId.set(null);
  vi.restoreAllMocks();
});

describe('drag drop handlers', () => {
  test('reorients a left/right split into top/bottom on a bottom-zone drop', () => {
    layoutTree.set(horizontalTree());
    const reinitializeTerminals = vi.fn();
    const { handleDrop } = createDragDropHandlers({ reinitializeTerminals });

    // Drag terminal 1 onto the bottom half of terminal 2.
    handleDrop({
      preventDefault: vi.fn(),
      dataTransfer: { getData: () => '1' },
    }, 2, 'bottom');

    const tree = get(layoutTree);
    expect(tree.direction).toBe('vertical');
    expect(tree.children.map((child) => child.terminalId)).toEqual([2, 1]);
    expect(reinitializeTerminals).toHaveBeenCalledOnce();
  });

  test('places the dragged pane first when dropping on the top zone', () => {
    layoutTree.set(horizontalTree());
    const { handleDrop } = createDragDropHandlers({ reinitializeTerminals: vi.fn() });

    handleDrop({ preventDefault: vi.fn(), dataTransfer: { getData: () => '1' } }, 2, 'top');

    const tree = get(layoutTree);
    expect(tree.direction).toBe('vertical');
    expect(tree.children.map((child) => child.terminalId)).toEqual([1, 2]);
  });

  test('falls back to swapping when no zone is provided', () => {
    layoutTree.set(horizontalTree());
    const { handleDrop } = createDragDropHandlers({ reinitializeTerminals: vi.fn() });

    handleDrop({ preventDefault: vi.fn(), dataTransfer: { getData: () => '1' } }, 2);

    const tree = get(layoutTree);
    expect(tree.direction).toBe('horizontal');
    expect(tree.children.map((child) => child.terminalId)).toEqual([2, 1]);
  });

  test('ignores a drop onto the same terminal', () => {
    layoutTree.set(horizontalTree());
    const reinitializeTerminals = vi.fn();
    const { handleDrop } = createDragDropHandlers({ reinitializeTerminals });

    handleDrop({ preventDefault: vi.fn(), dataTransfer: { getData: () => '2' } }, 2, 'bottom');

    expect(get(layoutTree)).toEqual(horizontalTree());
    expect(reinitializeTerminals).not.toHaveBeenCalled();
  });

  test('tracks the dragged terminal id in the shared store', () => {
    const documentRef = () => ({ querySelectorAll: () => [] });
    const { handleDragStart, handleDragEnd } = createDragDropHandlers({ documentRef });

    handleDragStart({
      dataTransfer: { setData: vi.fn(), effectAllowed: '' },
      currentTarget: { classList: classList() },
    }, 7);
    expect(get(draggingTerminalId)).toBe(7);

    handleDragEnd({ target: { classList: classList() } });
    expect(get(draggingTerminalId)).toBe(null);
  });
});
