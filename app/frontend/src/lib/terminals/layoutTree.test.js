import { describe, expect, test } from 'vitest';
import { appendLeaf, collectLeafIds, rebuildLayout, removeLeafFromTree, serializeLayout } from './layoutTree.js';

const leaf = (id) => ({ type: 'leaf', terminalId: id });

// Width of each top-level pane for a root chain, as a fraction of the area.
function widths(node, share = 1) {
  if (!node) return [];
  if (node.type === 'leaf') return [share];
  return [...widths(node.children[0], share * node.ratio), ...widths(node.children[1], share * (1 - node.ratio))];
}

describe('appendLeaf', () => {
  test('keeps every pane at an equal share instead of halving the tree', () => {
    let tree = null;
    for (let i = 1; i <= 6; i++) tree = appendLeaf(tree, leaf(i));
    const w = widths(tree);
    expect(w).toHaveLength(6);
    for (const x of w) expect(x).toBeCloseTo(1 / 6, 5);
    expect(collectLeafIds(tree)).toEqual([1, 2, 3, 4, 5, 6]);
  });

  test('a vertical root becomes one column of a two-pane row', () => {
    const tree = appendLeaf({ type: 'split', direction: 'vertical', ratio: 0.5, children: [leaf(1), leaf(2)] }, leaf(3));
    expect(tree.direction).toBe('horizontal');
    expect(tree.ratio).toBeCloseTo(0.5, 5);
  });
});

describe('layout persistence', () => {
  const keys = { 1: 'r1', 2: 'r2', 3: 'r3' };
  test('round-trips a nested layout through stable keys', () => {
    const tree = { type: 'split', direction: 'horizontal', ratio: 0.3, children: [leaf(1), { type: 'split', direction: 'vertical', ratio: 0.6, children: [leaf(2), leaf(3)] }] };
    const saved = serializeLayout(tree, (id) => keys[id]);
    expect(saved).toEqual({ type: 'split', direction: 'horizontal', ratio: 0.3, children: [{ type: 'leaf', key: 'r1' }, { type: 'split', direction: 'vertical', ratio: 0.6, children: [{ type: 'leaf', key: 'r2' }, { type: 'leaf', key: 'r3' }] }] });
    const ids = { r1: 11, r2: 22, r3: 33 };
    const back = rebuildLayout(JSON.parse(JSON.stringify(saved)), (k) => ids[k]);
    expect(collectLeafIds(back)).toEqual([11, 22, 33]);
    expect(back.children[1].direction).toBe('vertical');
    expect(back.children[1].ratio).toBeCloseTo(0.6, 5);
  });

  test('drops leaves that did not come back and clamps ratios', () => {
    const saved = { type: 'split', direction: 'horizontal', ratio: 5, children: [{ type: 'leaf', key: 'gone' }, { type: 'split', direction: 'vertical', ratio: 0.4, children: [{ type: 'leaf', key: 'a' }, { type: 'leaf', key: 'b' }] }] };
    const back = rebuildLayout(saved, (k) => ({ a: 1, b: 2 })[k] ?? null);
    expect(collectLeafIds(back)).toEqual([1, 2]);
    expect(back.direction).toBe('vertical');
    expect(rebuildLayout({ type: 'leaf', key: 'gone' }, () => null)).toBeNull();
    expect(rebuildLayout('garbage', () => 1)).toBeNull();
    expect(serializeLayout(leaf(9), () => null)).toBeNull();
  });
});

describe('closing a pane redistributes its share', () => {
  test('a row of three equal panes stays equal after one closes', () => {
    let tree = null;
    for (const id of [1, 2, 3, 4]) tree = appendLeaf(tree, { type: 'leaf', terminalId: id });
    tree = removeLeafFromTree(tree, 2);
    // [[1, 3], 4]: the outer split gives 4 one third, the inner split halves the rest.
    expect(tree.ratio).toBeCloseTo(2 / 3);
    expect(tree.children[0].ratio).toBeCloseTo(1 / 2);
    expect(collectLeafIds(tree)).toEqual([1, 3, 4]);
  });

  test('a split of another direction keeps its own ratio', () => {
    const tree = {
      type: 'split', direction: 'horizontal', ratio: 0.5,
      children: [
        { type: 'split', direction: 'vertical', ratio: 0.3, children: [{ type: 'leaf', terminalId: 1 }, { type: 'leaf', terminalId: 2 }] },
        { type: 'split', direction: 'horizontal', ratio: 0.5, children: [{ type: 'leaf', terminalId: 3 }, { type: 'leaf', terminalId: 4 }] },
      ],
    };
    const next = removeLeafFromTree(tree, 4);
    expect(collectLeafIds(next)).toEqual([1, 2, 3]);
    expect(next.children[0].ratio).toBeCloseTo(0.3);
  });
});
