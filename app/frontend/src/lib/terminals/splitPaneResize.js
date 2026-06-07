export function clampSplitRatio(value) {
  return Math.max(0.1, Math.min(0.9, value));
}

export function calculateSplitRatio(direction, rect, pointer) {
  if (!rect || !pointer) return 0.5;
  if (direction === 'horizontal') {
    return clampSplitRatio((pointer.clientX - rect.left) / rect.width);
  }
  return clampSplitRatio((pointer.clientY - rect.top) / rect.height);
}
