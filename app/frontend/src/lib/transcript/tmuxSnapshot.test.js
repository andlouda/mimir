import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { createSnapshotScheduler } from './tmuxSnapshot.js';

describe('transcript snapshot scheduler', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  test('snapshots each pane with output once per interval', async () => {
    const snapshot = vi.fn(async () => true);
    const s = createSnapshotScheduler({ snapshot, intervalMs: 1000 });
    s.markDirty(1);
    s.markDirty(1);
    s.markDirty(2);
    expect(snapshot).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1001);
    expect(snapshot.mock.calls.map((c) => c[0])).toEqual([1, 2]);
    // Quiet panes cost nothing: no timer, no further calls.
    await vi.advanceTimersByTimeAsync(5000);
    expect(snapshot).toHaveBeenCalledTimes(2);
    expect(vi.getTimerCount()).toBe(0);
    s.markDirty(2);
    await vi.advanceTimersByTimeAsync(1001);
    expect(snapshot).toHaveBeenCalledTimes(3);
  });

  test('flush snapshots at once and clears the pending mark', async () => {
    const snapshot = vi.fn(async () => true);
    const s = createSnapshotScheduler({ snapshot, intervalMs: 1000 });
    s.markDirty(7);
    expect(await s.flush(7)).toBe(true);
    expect(snapshot).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1001);
    expect(snapshot).toHaveBeenCalledTimes(1);
  });

  test('a failing snapshot does not stop the others', async () => {
    const snapshot = vi.fn(async (id) => { if (id === 1) throw new Error('host gone'); return true; });
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const s = createSnapshotScheduler({ snapshot, intervalMs: 1000 });
    s.markDirty(1);
    s.markDirty(2);
    await vi.advanceTimersByTimeAsync(1001);
    expect(snapshot).toHaveBeenCalledTimes(2);
    expect(await s.flush(1)).toBe(false);
    warn.mockRestore();
  });

  test('a forgotten pane is not snapshotted', async () => {
    const snapshot = vi.fn(async () => true);
    const s = createSnapshotScheduler({ snapshot, intervalMs: 1000 });
    s.markDirty(3);
    s.forget(3);
    await vi.advanceTimersByTimeAsync(1001);
    expect(snapshot).not.toHaveBeenCalled();
  });
});
