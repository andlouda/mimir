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
