// Deterministic wheel-to-lines conversion for terminal scrolling.
//
// Browsers (especially WebKitGTK on Linux) deliver wheel events with small
// pixel deltas for touchpads and smooth-scrolling mice. Truncating each event
// individually drops those deltas entirely, which makes scrolling feel dead or
// jumpy. Instead we keep a fractional remainder per terminal and only emit
// whole lines once enough delta has accumulated.

const DOM_DELTA_LINE = 1;
const DOM_DELTA_PAGE = 2;

// WeakMap so the remainder dies together with the terminal object.
const remainders = new WeakMap();

export function wheelEventToLines(event, lineHeightPx, pageRows) {
  if (event.deltaMode === DOM_DELTA_LINE) {
    return event.deltaY;
  }
  if (event.deltaMode === DOM_DELTA_PAGE) {
    return event.deltaY * pageRows;
  }
  const height = lineHeightPx > 0 ? lineHeightPx : 1;
  return event.deltaY / height;
}

export function consumeWheelEvent(key, event, lineHeightPx, pageRows) {
  const previous = remainders.get(key) || 0;
  const total = previous + wheelEventToLines(event, lineHeightPx, pageRows);
  const lines = Math.trunc(total) || 0; // normalize -0
  remainders.set(key, total - lines);
  return lines;
}

export function resetWheelRemainder(key) {
  remainders.delete(key);
}

// Key codes tmux understands as Shift+PageUp / Shift+PageDown (xterm
// encoding). In the "invisible" tmux integration these are bound to
// copy-mode scrolling, so wheel events can drive tmux history without tmux
// owning the mouse. One key scrolls three lines; scrolling up sends one
// extra S-PPage first, which enters copy-mode (harmless when already in it).
const TMUX_SCROLL_UP_KEY = '\x1b[5;2~';
const TMUX_SCROLL_DOWN_KEY = '\x1b[6;2~';
const TMUX_LINES_PER_KEY = 3;

export function tmuxScrollKeys(lines) {
  const n = Math.trunc(lines) || 0;
  if (n === 0) return '';
  const steps = Math.max(1, Math.ceil(Math.abs(n) / TMUX_LINES_PER_KEY));
  if (n < 0) return TMUX_SCROLL_UP_KEY.repeat(steps + 1);
  return TMUX_SCROLL_DOWN_KEY.repeat(steps);
}
