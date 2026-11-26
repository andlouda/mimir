// Pane placement kept on the host (see app/app_pane_meta.go).
//
// The window's split tree cannot live on one host: its panes belong to
// several hosts and to this machine. So each pane carries its own way from
// the root of the tree, a list of steps { d, r, i }: split direction, ratio
// of the first child, and which child the pane is under. The steps of the
// panes that come back are merged into a tree again; branches whose panes
// did not come back collapse.

export const PANE_META_VERSION = 1;

/** Map of terminal id → path from the root of the split tree. */
export function leafPaths(tree) {
  const paths = new Map();
  (function walk(node, path) {
    if (!node) return;
    if (node.type === 'leaf') {
      paths.set(node.terminalId, path);
      return;
    }
    if (node.type !== 'split' || !Array.isArray(node.children) || node.children.length !== 2) return;
    const step = { d: node.direction === 'vertical' ? 'v' : 'h', r: roundRatio(node.ratio) };
    walk(node.children[0], [...path, { ...step, i: 0 }]);
    walk(node.children[1], [...path, { ...step, i: 1 }]);
  })(tree, []);
  return paths;
}

function roundRatio(ratio) {
  const value = Number(ratio);
  if (!Number.isFinite(value)) return 0.5;
  return Math.round(Math.min(0.9, Math.max(0.1, value)) * 1000) / 1000;
}

/** What a pane stores on its session: its name and its place. */
export function paneMetaFor(term, path) {
  return { v: PANE_META_VERSION, name: String(term?.name || '').slice(0, 80), path: path || [] };
}

/**
 * Merges the paths of the panes that came back into one tree.
 * entries: [{ id, path }]. Returns null when there is nothing to build or
 * the paths contradict each other (saved at different times, for instance);
 * the caller then keeps its plain arrangement.
 */
export function treeFromPaths(entries) {
  const usable = (entries || []).filter((entry) => entry && entry.id != null && Array.isArray(entry.path));
  if (usable.length === 0) return null;
  const tree = build(usable, 0);
  return tree === INVALID ? null : tree;
}

const INVALID = Symbol('invalid');

function build(entries, depth) {
  if (entries.length === 1) return { type: 'leaf', terminalId: entries[0].id };
  // Two panes cannot sit in the same place.
  if (entries.some((entry) => entry.path.length <= depth)) return INVALID;
  const first = entries[0].path[depth];
  if (entries.some((entry) => entry.path[depth].d !== first.d)) return INVALID;
  const left = entries.filter((entry) => entry.path[depth].i === 0);
  const right = entries.filter((entry) => entry.path[depth].i !== 0);
  if (left.length === 0) return build(right, depth + 1);
  if (right.length === 0) return build(left, depth + 1);
  const a = build(left, depth + 1);
  const b = build(right, depth + 1);
  if (a === INVALID || b === INVALID) return INVALID;
  return { type: 'split', direction: first.d === 'v' ? 'vertical' : 'horizontal', ratio: roundRatio(first.r), children: [a, b] };
}
