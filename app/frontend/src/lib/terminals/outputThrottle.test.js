import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { createOutputThrottle, INACTIVE_FLUSH_MS } from './outputThrottle.js';

describe('inactive pane output throttle', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  test('active panes write at once, inactive ones in batches', () => {
    const writes = [];
    let active = 1;
    const t = createOutputThrottle({ write: (term, d) => writes.push([term.id, d]), isImmediate: (term) => term.id === active || term.minimized });
    t.push({ id: 1 }, 'a');
    t.push({ id: 2 }, 'b');
    t.push({ id: 2 }, 'c');
    t.push({ id: 3, minimized: true }, 'm');
    expect(writes).toEqual([[1, 'a'], [3, 'm']]);
    expect(t.pending(2)).toBe(2);
    vi.advanceTimersByTime(INACTIVE_FLUSH_MS);
    expect(writes).toEqual([[1, 'a'], [3, 'm'], [2, 'bc']]);
  });

  test('activating a pane flushes its backlog before new output', () => {
    const writes = [];
    let active = 1;
    const t = createOutputThrottle({ write: (term, d) => writes.push([term.id, d]), isImmediate: (term) => term.id === active });
    t.push({ id: 2 }, 'old');
    active = 2;
    t.push({ id: 2 }, 'new');
    expect(writes).toEqual([[2, 'old'], [2, 'new']]);
    t.push({ id: 5 }, 'x');
    t.forget(5);
    vi.advanceTimersByTime(200);
    expect(writes.length).toBe(2);
  });

  test('a large backlog flushes early', () => {
    const writes = [];
    const t = createOutputThrottle({ write: (term, d) => writes.push(d.length), isImmediate: () => false });
    t.push({ id: 2 }, 'x'.repeat(300 * 1024));
    expect(writes).toEqual([]);
    t.push({ id: 2 }, 'y'.repeat(300 * 1024));
    expect(writes).toEqual([600 * 1024]);
  });

  test('interval may be a function read at scheduling time', () => {
    const timers = [];
    let interval = 100;
    const t = createOutputThrottle({
      write: () => {}, isImmediate: () => false, intervalMs: () => interval,
      setTimer: (fn, ms) => { timers.push(ms); return 1; }, clearTimer: () => {},
    });
    t.push({ id: 1 }, 'a');
    interval = 30000;
    t.push({ id: 2 }, 'b');
    expect(timers).toEqual([100, 30000]);
  });
});
