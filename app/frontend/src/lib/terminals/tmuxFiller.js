// tmux draws the part of the client that lies outside a smaller window as a
// border (│ on the right, ─ and ┘ below) filled with dots (·). xterm only
// sees the drawn cells, so every row of such a pane ends in
// "<spaces>│······" and rows below the window are pure filler. Link
// detection and copying strip that filler and use the window width (the
// column of │) instead of the pane width.

const RIGHT_FILLER = /│·+\s*$/;
const FILLER_ROW = /^\s*(?:─*┘)?·+\s*$|^\s*─+┘\s*$/;

/**
 * @param {string} raw  one terminal row
 * @param {number} cols pane width
 * @returns {{text: string, width: number, filler: boolean}} text without
 *   filler and trailing blanks; width is the tmux window width when filler
 *   was found, else cols
 */
export function stripTmuxFiller(raw, cols = 0) {
  const row = String(raw ?? '');
  if (FILLER_ROW.test(row)) return { text: '', width: cols, filler: true };
  const m = RIGHT_FILLER.exec(row);
  if (!m) return { text: row.trimEnd(), width: cols, filler: false };
  return { text: row.slice(0, m.index).trimEnd(), width: m.index, filler: true };
}

/**
 * Removes tmux's filler from a copied selection: the border and dots at the
 * end of each row, the padding before them and filler rows below the window.
 */
export function cleanTerminalSelection(text) {
  const lines = String(text ?? '').replace(/\r/g, '').split('\n');
  const out = [];
  let sawFiller = false;
  for (const line of lines) {
    const info = stripTmuxFiller(line);
    if (info.filler) sawFiller = true;
    if (info.filler && info.text === '' && FILLER_ROW.test(line)) continue;
    out.push(info.filler ? info.text : line);
  }
  if (!sawFiller) return String(text ?? '');
  return out.join('\n');
}
