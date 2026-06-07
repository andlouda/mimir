import { describe, expect, test } from 'vitest';
import { calculateSplitRatio, clampSplitRatio } from './splitPaneResize.js';

describe('split pane resize', () => {
  test('clamps ratios to the supported pane bounds', () => {
    expect(clampSplitRatio(-1)).toBe(0.1);
    expect(clampSplitRatio(0.42)).toBe(0.42);
    expect(clampSplitRatio(2)).toBe(0.9);
  });

  test('calculates horizontal split ratio from pointer x position', () => {
    const ratio = calculateSplitRatio(
      'horizontal',
      { left: 100, top: 0, width: 400, height: 200 },
      { clientX: 300, clientY: 999 },
    );
    expect(ratio).toBe(0.5);
  });

  test('calculates vertical split ratio from pointer y position', () => {
    const ratio = calculateSplitRatio(
      'vertical',
      { left: 0, top: 20, width: 400, height: 300 },
      { clientX: 999, clientY: 95 },
    );
    expect(ratio).toBe(0.25);
  });

  test('clamps dragged pointer outside the container', () => {
    expect(calculateSplitRatio('horizontal', { left: 0, width: 400 }, { clientX: -200 })).toBe(0.1);
    expect(calculateSplitRatio('vertical', { top: 0, height: 400 }, { clientY: 800 })).toBe(0.9);
  });
});
