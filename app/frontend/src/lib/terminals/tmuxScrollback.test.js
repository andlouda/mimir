import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { createScrollbackRefill } from './tmuxScrollback.js';

function fakeXterm(rows = 3) {
  const writes = [];
  return {
    rows,
    writes,
    cleared: 0,
    write(data, cb) { writes.push(data); cb?.(); },
    clear() { this.cleared++; },
    scrollToBottom() {},
  };
}

describe('tmux scrollback refill', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  test('re-fills the scrollback after a widen settles and repaints', async () => {
    const xterm = fakeXterm(2);
    const fetchHistory = vi.fn(async () => ({ alternate: false, lines: ['one', 'two'] }));
    const refreshClient = vi.fn(async () => {});
    const refill = createScrollbackRefill({ fetchHistory, refreshClient });
    const term = { id: 1, terminal: xterm, tmuxActive: true };
    refill.onResized(term, 80, 120);
    expect(fetchHistory).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(refill._settleMs + 10);
    expect(fetchHistory).toHaveBeenCalledWith(1, 5000);
    expect(xterm.cleared).toBe(1);
    expect(xterm.writes[0]).toBe('\x1b[2J\x1b[H');
    expect(xterm.writes[1]).toBe('one\r\ntwo\x1b[0m\r\n\r\n\r\n');
    expect(refreshClient).toHaveBeenCalledWith(1);
  });

  test('skips narrowing, non-tmux panes, alternate screens and a disabled setting', async () => {
    const fetchHistory = vi.fn(async () => ({ alternate: true, lines: ['x'] }));
    const refreshClient = vi.fn(async () => {});
    let enabled = true;
    const refill = createScrollbackRefill({ fetchHistory, refreshClient, isEnabled: () => enabled });
    const xterm = fakeXterm();
    refill.onResized({ id: 1, terminal: xterm, tmuxActive: true }, 120, 80);
    refill.onResized({ id: 2, terminal: xterm, tmuxActive: false }, 80, 120);
    enabled = false;
    refill.onResized({ id: 3, terminal: xterm, tmuxActive: true }, 80, 120);
    enabled = true;
    await vi.advanceTimersByTimeAsync(1000);
    expect(fetchHistory).not.toHaveBeenCalled();
    refill.onResized({ id: 4, terminal: xterm, tmuxActive: true }, 80, 120);
    await vi.advanceTimersByTimeAsync(1000);
    expect(fetchHistory).toHaveBeenCalledTimes(1);
    expect(xterm.cleared).toBe(0); // alternate screen: nothing touched
    expect(refreshClient).not.toHaveBeenCalled();
  });

  test('a second resize while waiting cancels the first', async () => {
    const fetchHistory = vi.fn(async () => ({ alternate: false, lines: ['a'] }));
    const refill = createScrollbackRefill({ fetchHistory, refreshClient: async () => {} });
    const term = { id: 1, terminal: fakeXterm(), tmuxActive: true };
    refill.onResized(term, 80, 100);
    await vi.advanceTimersByTimeAsync(300);
    refill.onResized(term, 100, 120);
    await vi.advanceTimersByTimeAsync(1000);
    expect(fetchHistory).toHaveBeenCalledTimes(1);
  });
});
