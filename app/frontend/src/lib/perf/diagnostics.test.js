import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { countOutput, countWrite, perfStats, startDiagnostics, stopDiagnostics } from './diagnostics.js';

describe('diagnostics sampler', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => { stopDiagnostics(); vi.useRealTimers(); });

  test('folds counters into a per-second snapshot and clears on stop', () => {
    startDiagnostics({ getPanes: () => 9, getAnimations: () => 0 });
    for (let i = 0; i < 30; i++) countOutput(1024);
    countWrite(); countWrite();
    vi.advanceTimersByTime(1000);
    const s = get(perfStats);
    expect(s.eventsPerSec).toBe(30);
    expect(s.kbPerSec).toBe(30);
    expect(s.writesPerSec).toBe(2);
    expect(s.panes).toBe(9);
    expect(s.mainThreadBusy).toBeGreaterThanOrEqual(0);
    stopDiagnostics();
    expect(get(perfStats)).toBe(null);
  });
});
