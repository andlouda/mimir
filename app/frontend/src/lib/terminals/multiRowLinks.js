// URL detection that survives program-drawn line breaks.
//
// xterm's stock link detection works per buffer line. Programs like Claude
// Code wrap long URLs themselves (a full-width row, then a continuation row
// with indentation), so only the first row is recognised. This finder treats
// a row that fills the terminal width and ends inside a URL as continued on
// the next row, joins the parts and reports one link spanning all rows.
//
// Two more shapes are recognised:
//   - a tmux window smaller than the pane: rows end in tmux's border and dot
//     filler (see tmuxFiller.js); "full" then means the window width.
//   - a URL hard-wrapped at a fixed width narrower than the terminal (fold,
//     programs printing at 80 columns): rows of equal length that end inside
//     the URL, followed by rows that hold nothing but URL characters.

import { stripTmuxFiller } from './tmuxFiller.js';

const URL_CHARS = "A-Za-z0-9\\-._~:/?#\\[\\]@!$&'()*+,;=%";
const URL_START = new RegExp(`https?://[${URL_CHARS}]+`, 'g');
const CONTINUATION = new RegExp(`^( {0,8})([${URL_CHARS}]+)`);
const TRAILING_PUNCT = /[.,;:!?'"]+$/;
const URL_ONLY = new RegExp(`^[${URL_CHARS}]+$`);
const MAX_ROWS = 12;
const MIN_FOLD_WIDTH = 40;

/**
 * @param {(y:number) => string|null} rawRowText  text of buffer row y (1-based), null when absent
 * @param {number} y      buffer row to provide links for
 * @param {number} cols   terminal width
 * @returns {Array<{text:string, start:{x:number,y:number}, end:{x:number,y:number}}>}
 */
export function findLinksAt(rawRowText, y, cols) {
  const cache = new Map();
  const info = (row) => {
    if (!cache.has(row)) {
      const raw = rawRowText(row);
      cache.set(row, raw == null ? null : stripTmuxFiller(raw, cols));
    }
    return cache.get(row);
  };
  const rowText = (row) => info(row)?.text ?? null;
  const isFull = (row) => {
    const i = info(row);
    return i != null && i.width > 0 && i.text.length >= i.width;
  };
  // Hard wrap at a fixed width W: the row is W long, the next row is URL
  // characters only (no indent), and W is confirmed by a neighbour of the
  // same length or is fold's default of 80.
  const isFolded = (row) => {
    const t = rowText(row);
    const next = rowText(row + 1);
    if (t == null || next == null || t.length < MIN_FOLD_WIDTH || !URL_ONLY.test(next)) return false;
    return next.length === t.length || rowText(row - 1)?.length === t.length || t.length === 80;
  };
  const continues = (row, nextText) => {
    if (isFull(row)) return CONTINUATION.exec(nextText);
    if (isFolded(row)) return /^()(.*)$/.exec(nextText);
    return null;
  };

  // Walk back to rows that might hold the start of a URL continued onto y.
  let start = y;
  while (y - start < MAX_ROWS) {
    const text = rowText(start);
    if (text == null || !continues(start - 1, text)) break;
    start -= 1;
  }

  const links = [];
  for (let row = start; row <= y; row++) {
    const text = rowText(row);
    if (!text) continue;
    URL_START.lastIndex = 0;
    let match;
    while ((match = URL_START.exec(text)) !== null) {
      let url = match[0];
      let endRow = row;
      let endX = match.index + url.length; // 1-based inclusive column of last char
      // Continue across rows while the current row is full and ended inside the URL.
      let cur = row;
      while (endX === rowText(cur).length && endRow - row < MAX_ROWS) {
        const next = rowText(cur + 1);
        const cont = next == null ? null : continues(cur, next);
        if (!cont) break;
        url += cont[2];
        endRow = cur + 1;
        endX = cont[1].length + cont[2].length;
        cur += 1;
      }
      const trimmed = url.replace(TRAILING_PUNCT, '');
      if (trimmed.length !== url.length) {
        // Trailing punctuation belongs to the prose; shorten the range too.
        const cut = url.length - trimmed.length;
        url = trimmed;
        endX -= cut;
        if (endX <= 0) continue;
      }
      if (row <= y && endRow >= y) {
        links.push({ text: url, start: { x: match.index + 1, y: row }, end: { x: endX, y: endRow } });
      }
    }
  }
  return links;
}
