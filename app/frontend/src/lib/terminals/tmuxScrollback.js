// tmux stores its history at the width it was written. After a pane is
// widened, lines scrolled off earlier stay cut at the old width, and xterm
// cannot re-wrap them (tmux emitted hard line breaks). This module re-fills
// xterm's scrollback from tmux once a widen has settled: the history comes
// back with tmux's soft wraps joined, so xterm wraps it at the new width,
// then tmux repaints the visible screen.

const SETTLE_MS = 700;
const HISTORY_LINES = 5000;

export function createScrollbackRefill({ fetchHistory, refreshClient, isEnabled = () => true, now = () => Date.now() }) {
  const timers = new Map(); // term.id → timeout
  const generations = new Map(); // term.id → counter, invalidates a refill in flight

  function cancel(id) {
    const t = timers.get(id);
    if (t) clearTimeout(t);
    timers.delete(id);
    generations.set(id, (generations.get(id) || 0) + 1);
  }

  /** Call after a resize was sent to the PTY. Only a widen of a tmux pane qualifies. */
  function onResized(term, prevCols, cols) {
    if (!term) return;
    cancel(term.id);
    if (!isEnabled() || !term.tmuxActive || term.minimized || !(cols > prevCols)) return;
    const gen = generations.get(term.id) || 0;
    const timer = setTimeout(() => {
      timers.delete(term.id);
      refill(term, gen).catch((error) => console.warn(`tmux scrollback refill failed for terminal ${term.id}:`, error?.message || error));
    }, SETTLE_MS);
    timers.set(term.id, timer);
  }

  async function refill(term, gen) {
    const xterm = term.terminal;
    if (!xterm) return;
    const history = await fetchHistory(term.id, HISTORY_LINES);
    if ((generations.get(term.id) || 0) !== gen) return; // resized again meanwhile
    if (!history || history.alternate || !Array.isArray(history.lines) || history.lines.length === 0) return;
    // Empty the screen and the scrollback (clear() keeps the cursor line,
    // so the screen is wiped first), write the joined history, push it
    // above the viewport, then let tmux repaint the live screen.
    await new Promise((resolve) => xterm.write('\x1b[2J\x1b[H', resolve));
    xterm.clear();
    const body = history.lines.join('\r\n') + '\x1b[0m\r\n' + '\r\n'.repeat(Math.max(0, xterm.rows));
    await new Promise((resolve) => xterm.write(body, resolve));
    if ((generations.get(term.id) || 0) !== gen) return;
    await refreshClient(term.id);
    try { xterm.scrollToBottom(); } catch { /* ignore */ }
  }

  return { onResized, cancel, _settleMs: SETTLE_MS };
}
