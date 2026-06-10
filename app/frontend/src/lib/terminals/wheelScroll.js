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
