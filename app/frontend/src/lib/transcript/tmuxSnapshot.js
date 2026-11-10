// Keeps the width-independent transcript of tmux panes current.
//
// The raw transcript of a tmux pane is tmux painting the screen at the
// pane's width of the moment; it cannot be shown at one stable width. The
// backend can save tmux's own history as logical lines instead (see
// app/transcript/history.go), but only while the session lives. This
// scheduler asks for that snapshot some time after a pane produced output,
// so a transcript read later (even after the session is gone) is at most
// one interval behind.

export const SNAPSHOT_INTERVAL_MS = 5 * 60 * 1000;

/**
 * @param {object} deps
 * @param {(id:number) => Promise<unknown>} deps.snapshot  saves one pane's history; may reject
 * @param {number} [deps.intervalMs]
 */
export function createSnapshotScheduler({ snapshot, intervalMs = SNAPSHOT_INTERVAL_MS }) {
  const dirty = new Set();
  let timer = null;

  async function tick() {
    timer = null;
    const ids = [...dirty];
    dirty.clear();
    // One after the other: several panes often share one host.
    for (const id of ids) {
      try {
        await snapshot(id);
      } catch (error) {
        console.warn(`transcript snapshot failed for terminal ${id}:`, error?.message || error);
      }
    }
  }

  /** The pane produced output; snapshot it once the interval has passed. */
  function markDirty(id) {
    dirty.add(id);
    // The timer only exists while something is waiting, so an idle app
    // keeps no timer running.
    if (timer == null) timer = setTimeout(tick, intervalMs);
  }

  /** Snapshot now (pane about to close, viewer opening); resolves when done. */
  async function flush(id) {
    dirty.delete(id);
    try {
      return await snapshot(id);
    } catch (error) {
      console.warn(`transcript snapshot failed for terminal ${id}:`, error?.message || error);
      return false;
    }
  }

  function forget(id) {
    dirty.delete(id);
  }

  return { markDirty, flush, forget, _pending: () => dirty.size };
}
