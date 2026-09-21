// URL detection that survives program-drawn line breaks.
//
// xterm's stock link detection works per buffer line. Programs like Claude
// Code wrap long URLs themselves (a full-width row, then a continuation row
// with indentation), so only the first row is recognised. This finder treats
// a row that fills the terminal width and ends inside a URL as continued on
// the next row, joins the parts and reports one link spanning all rows.

const URL_CHARS = "A-Za-z0-9\\-._~:/?#\\[\\]@!$&'()*+,;=%";
const URL_START = new RegExp(`https?://[${URL_CHARS}]+`, 'g');
const CONTINUATION = new RegExp(`^( {0,8})([${URL_CHARS}]+)`);
const TRAILING_PUNCT = /[.,;:!?'"]+$/;
const MAX_ROWS = 12;

/**
 * @param {(y:number) => string|null} rowText  right-trimmed text of buffer row y (1-based), null when absent
 * @param {number} y      buffer row to provide links for
 * @param {number} cols   terminal width
 * @returns {Array<{text:string, start:{x:number,y:number}, end:{x:number,y:number}}>}
 */
export function findLinksAt(rowText, y, cols) {
  const isFull = (row) => {
    const t = rowText(row);
    return t != null && cols > 0 && t.length >= cols;
  };

  // Walk back to rows that might hold the start of a URL continued onto y.
  let start = y;
  while (y - start < MAX_ROWS) {
    const text = rowText(start);
    if (text == null || !CONTINUATION.test(text) || !isFull(start - 1)) break;
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
      while (endX === rowText(cur).length && isFull(cur) && endRow - row < MAX_ROWS) {
        const next = rowText(cur + 1);
        const cont = next == null ? null : CONTINUATION.exec(next);
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
