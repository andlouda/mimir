// Panes that are visible but not active still get every output chunk
// drawn at once; with several redrawing TUIs (Claude Code, OpenCode)
// that is where the webview's CPU goes. For inactive panes the output is
// collected and written a few times per second instead: xterm then does
// one parse + render per batch. The active pane and minimized panes
// (not attached, nothing to draw) are written immediately. Activating a
// pane flushes its backlog first.

export const INACTIVE_FLUSH_MS = 100;
const BACKLOG_MAX = 512 * 1024;

export function createOutputThrottle({ write, isImmediate, intervalMs = INACTIVE_FLUSH_MS, setTimer = setTimeout, clearTimer = clearTimeout }) {
  const backlog = new Map(); // id → { term, chunks: [], size, timer }

  function flush(id) {
    const b = backlog.get(id);
    if (!b) return;
    backlog.delete(id);
    if (b.timer) clearTimer(b.timer);
    if (b.chunks.length) write(b.term, b.chunks.join(''));
  }

  function push(term, data) {
    if (!data) return;
    if (isImmediate(term)) {
      flush(term.id);
      write(term, data);
      return;
    }
    let b = backlog.get(term.id);
    if (!b) {
      b = { term, chunks: [], size: 0, timer: null };
      backlog.set(term.id, b);
      b.timer = setTimer(() => { b.timer = null; flush(term.id); }, intervalMs);
    }
    b.term = term;
    b.chunks.push(data);
    b.size += data.length;
    if (b.size >= BACKLOG_MAX) flush(term.id);
  }

  function forget(id) {
    const b = backlog.get(id);
    if (b?.timer) clearTimer(b.timer);
    backlog.delete(id);
  }

  return { push, flush, forget, pending: (id) => backlog.get(id)?.size || 0 };
}
