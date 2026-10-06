import { describe, expect, test } from 'vitest';
import { hasVisibleAgent, placementDirection } from './panePlacement.js';

describe('pane placement', () => {
  test('explicit modes ignore agents', () => {
    expect(placementDirection('row', { visibleAgent: true })).toBe('horizontal');
    expect(placementDirection('column', { visibleAgent: false })).toBe('vertical');
  });
  test('auto stacks only while an agent is visible', () => {
    expect(placementDirection('auto', { visibleAgent: false })).toBe('horizontal');
    expect(placementDirection('auto', { visibleAgent: true })).toBe('vertical');
    expect(placementDirection(undefined, { visibleAgent: true })).toBe('horizontal'); // default: side by side
  });
  test('minimized agent panes do not count', () => {
    const states = { 1: { kind: 'claude' } };
    expect(hasVisibleAgent([{ id: 1, minimized: true }], states)).toBe(false);
    expect(hasVisibleAgent([{ id: 1 }, { id: 2 }], states)).toBe(true);
    expect(hasVisibleAgent([{ id: 2 }], states)).toBe(false);
  });
});
