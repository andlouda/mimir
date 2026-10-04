export function replaceLeaf(node, terminalId, replacement) {
  if (!node) return replacement;
  if (node.type === 'leaf') {
    return node.terminalId === terminalId ? replacement : node;
  }
  return {
    ...node,
    children: [
      replaceLeaf(node.children[0], terminalId, replacement),
      replaceLeaf(node.children[1], terminalId, replacement)
    ]
  };
}

export function removeLeafFromTree(node, terminalId) {
  if (!node) return null;
  if (node.type === 'leaf') {
    return node.terminalId === terminalId ? null : node;
  }
  const left = node.children[0];
  const right = node.children[1];
  if (left.type === 'leaf' && left.terminalId === terminalId) return right;
  if (right.type === 'leaf' && right.terminalId === terminalId) return left;
  const newLeft = removeLeafFromTree(left, terminalId);
  const newRight = removeLeafFromTree(right, terminalId);
  if (newLeft === null) return newRight;
  if (newRight === null) return newLeft;
  return { ...node, children: [newLeft, newRight] };
}

export function collectLeafIds(node) {
  if (!node) return [];
  if (node.type === 'leaf') return [node.terminalId];
  return [...collectLeafIds(node.children[0]), ...collectLeafIds(node.children[1])];
}

// moveLeaf relocates draggedId next to targetId, choosing the split orientation
// from the drop position: 'left'/'right' produce a side-by-side (horizontal)
// split, 'top'/'bottom' a stacked (vertical) one. Unlike swapLeaves this is a
// real move — the dragged leaf is detached from wherever it was and re-attached
// beside the target, which is what lets a left/right layout become top/bottom.
export function moveLeaf(node, draggedId, targetId, position) {
  if (!node || draggedId === targetId) return node;
  const ids = collectLeafIds(node);
  if (!ids.includes(draggedId) || !ids.includes(targetId)) return node;

  const withoutDragged = removeLeafFromTree(node, draggedId);
  if (!withoutDragged) return node;

  const direction = position === 'left' || position === 'right' ? 'horizontal' : 'vertical';
  const draggedFirst = position === 'left' || position === 'top';
  const draggedLeaf = { type: 'leaf', terminalId: draggedId };
  const targetLeaf = { type: 'leaf', terminalId: targetId };
  const splitNode = {
    type: 'split',
    direction,
    ratio: 0.5,
    children: draggedFirst ? [draggedLeaf, targetLeaf] : [targetLeaf, draggedLeaf],
  };
  return replaceLeaf(withoutDragged, targetId, splitNode);
}

export function swapLeaves(node, idA, idB) {
  if (!node) return node;
  if (node.type === 'leaf') {
    if (node.terminalId === idA) return { ...node, terminalId: idB };
    if (node.terminalId === idB) return { ...node, terminalId: idA };
    return node;
  }
  return {
    ...node,
    children: [
      swapLeaves(node.children[0], idA, idB),
      swapLeaves(node.children[1], idA, idB)
    ]
  };
}

// ---- balanced append ------------------------------------------------------

// Leaves along the root chain of a split direction: for h[h[h[a,b],c],d]
// that is 4, for a leaf 1, for a root split of the other direction 1.
function chainCount(node, direction) {
  if (!node) return 0;
  if (node.type === 'leaf' || node.direction !== direction) return 1;
  return chainCount(node.children[0], direction) + 1;
}

// appendLeaf adds a pane at the end of the top-level row (or column) so that
// every pane in that row keeps an equal share. Wrapping the whole tree in a
// new 50 % split, as "+ New" used to, halves the existing panes each time
// (1/2, 1/4, 1/8 …); here the new root ratio is n/(n+1), which keeps the
// existing panes' proportions and gives the newcomer the same width as
// each of them when they were equal.
export function appendLeaf(node, leaf, direction = 'horizontal') {
  if (!node) return leaf;
  const n = chainCount(node, direction);
  return { type: 'split', direction, ratio: n / (n + 1), children: [node, leaf] };
}

// ---- persistence ----------------------------------------------------------

// serializeLayout replaces terminal ids with stable keys (keyFor(id) → string
// or null); leaves without a key are dropped. Returns null for an empty tree.
export function serializeLayout(node, keyFor) {
  if (!node) return null;
  if (node.type === 'leaf') {
    const key = keyFor(node.terminalId);
    return key ? { type: 'leaf', key } : null;
  }
  const left = serializeLayout(node.children[0], keyFor);
  const right = serializeLayout(node.children[1], keyFor);
  if (!left) return right;
  if (!right) return left;
  return { type: 'split', direction: node.direction === 'vertical' ? 'vertical' : 'horizontal', ratio: clamp01(node.ratio), children: [left, right] };
}

// rebuildLayout turns a serialised layout back into a tree with the ids of
// this run (idFor(key) → id or null). Leaves whose key did not come back are
// dropped; the caller appends terminals that are not in the layout.
export function rebuildLayout(saved, idFor) {
  if (!saved || typeof saved !== 'object') return null;
  if (saved.type === 'leaf') {
    const id = idFor(saved.key);
    return id == null ? null : { type: 'leaf', terminalId: id };
  }
  if (saved.type !== 'split' || !Array.isArray(saved.children) || saved.children.length !== 2) return null;
  const left = rebuildLayout(saved.children[0], idFor);
  const right = rebuildLayout(saved.children[1], idFor);
  if (!left) return right;
  if (!right) return left;
  return { type: 'split', direction: saved.direction === 'vertical' ? 'vertical' : 'horizontal', ratio: clamp01(saved.ratio), children: [left, right] };
}

function clamp01(r) {
  const v = Number(r);
  if (!Number.isFinite(v)) return 0.5;
  return Math.min(0.9, Math.max(0.1, v));
}
