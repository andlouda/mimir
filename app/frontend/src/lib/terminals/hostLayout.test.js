import { describe, expect, test } from 'vitest';
import { leafPaths, paneMetaFor, treeFromPaths } from './hostLayout.js';

const leaf = (terminalId) => ({ type: 'leaf', terminalId });
const split = (direction, ratio, a, b) => ({ type: 'split', direction, ratio, children: [a, b] });

// [ 1 | [ 2 / 3 ] | 4 ] as Mimir builds it: ((1 | (2 / 3)) | 4)
const tree = split('horizontal', 0.6667, split('horizontal', 0.5, leaf(1), split('vertical', 0.3, leaf(2), leaf(3))), leaf(4));

describe('pane placement kept on the host', () => {
  test('every pane knows its way from the root', () => {
    const paths = leafPaths(tree);
    expect(paths.get(1)).toEqual([{ d: 'h', r: 0.667, i: 0 }, { d: 'h', r: 0.5, i: 0 }]);
    expect(paths.get(3)).toEqual([{ d: 'h', r: 0.667, i: 0 }, { d: 'h', r: 0.5, i: 1 }, { d: 'v', r: 0.3, i: 1 }]);
    expect(paths.get(4)).toEqual([{ d: 'h', r: 0.667, i: 1 }]);
    expect(leafPaths(leaf(9)).get(9)).toEqual([]);
    expect(leafPaths(null).size).toBe(0);
  });

  test('all panes back: the same tree again', () => {
    const paths = leafPaths(tree);
    const rebuilt = treeFromPaths([4, 2, 1, 3].map((id) => ({ id, path: paths.get(id) })));
    expect(rebuilt).toEqual(split('horizontal', 0.667, split('horizontal', 0.5, leaf(1), split('vertical', 0.3, leaf(2), leaf(3))), leaf(4)));
  });

  test('branches whose panes did not come back collapse', () => {
    const paths = leafPaths(tree);
    // Panes 1 and 4 belong to another host: only 2 and 3 return.
    expect(treeFromPaths([2, 3].map((id) => ({ id, path: paths.get(id) })))).toEqual(split('vertical', 0.3, leaf(2), leaf(3)));
    // 1 and 3 return: 3 takes the place of the whole right branch.
    expect(treeFromPaths([1, 3].map((id) => ({ id, path: paths.get(id) })))).toEqual(split('horizontal', 0.5, leaf(1), leaf(3)));
    expect(treeFromPaths([{ id: 3, path: paths.get(3) }])).toEqual(leaf(3));
  });

  test('contradicting paths give no tree', () => {
    const same = [{ d: 'h', r: 0.5, i: 0 }];
    expect(treeFromPaths([{ id: 1, path: same }, { id: 2, path: same }])).toBeNull();
    expect(treeFromPaths([{ id: 1, path: [{ d: 'h', r: 0.5, i: 0 }] }, { id: 2, path: [{ d: 'v', r: 0.5, i: 1 }] }])).toBeNull();
    expect(treeFromPaths([{ id: 1, path: [] }, { id: 2, path: [] }])).toBeNull();
    expect(treeFromPaths([])).toBeNull();
    expect(treeFromPaths([{ id: 1 }])).toBeNull();
  });

  test('the stored value carries name and place', () => {
    expect(paneMetaFor({ name: 'SSH: Pironman' }, [{ d: 'h', r: 0.5, i: 1 }])).toEqual({ v: 1, name: 'SSH: Pironman', path: [{ d: 'h', r: 0.5, i: 1 }] });
    expect(paneMetaFor({ name: 'x'.repeat(200) }, undefined).name).toHaveLength(80);
  });
});
