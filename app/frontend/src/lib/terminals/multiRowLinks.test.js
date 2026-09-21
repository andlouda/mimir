import { describe, expect, test } from 'vitest';
import { findLinksAt } from './multiRowLinks.js';

function rows(lines) {
  return (y) => (y >= 1 && y <= lines.length ? lines[y - 1].replace(/\s+$/, '') : null);
}

describe('findLinksAt', () => {
  test('single-row URL, trailing punctuation dropped', () => {
    const r = rows(['see https://example.com/a?b=1. ok', 'next']);
    expect(findLinksAt(r, 1, 80)).toEqual([
      { text: 'https://example.com/a?b=1', start: { x: 5, y: 1 }, end: { x: 29, y: 1 } },
    ]);
    expect(findLinksAt(r, 2, 80)).toEqual([]);
  });

  test('URL wrapped by the program across five rows is one link', () => {
    const cols = 40;
    const expected = 'https://claude.ai/oauth/authorize?code=abcdef0123456789&redirect_uri=https%3A%2F%2Flocalhost%3A1234&state=xyz012345&scope=user%3Aprofile+org%3Acreate_api_key';
    // Claude Code style: 2-space indent, rows filled to the terminal width.
    const chunks = [];
    for (let i = 0; i < expected.length; i += cols - 2) chunks.push('  ' + expected.slice(i, i + cols - 2));
    const lines = ['● Browser didn’t open? Use this URL:', ...chunks, '', '  Paste code here if prompted >'];
    expect(chunks.length).toBe(5);
    chunks.slice(0, -1).forEach((c) => expect(c.length).toBe(cols));
    const r = rows(lines);
    const lastRow = 1 + chunks.length;
    for (let y = 2; y <= lastRow; y++) {
      const links = findLinksAt(r, y, cols);
      expect(links, `row ${y}`).toHaveLength(1);
      expect(links[0].text).toBe(expected);
      expect(links[0].start).toEqual({ x: 3, y: 2 });
      expect(links[0].end).toEqual({ x: chunks[chunks.length - 1].length, y: lastRow });
    }
    expect(findLinksAt(r, 1, cols)).toEqual([]);
    expect(findLinksAt(r, lastRow + 2, cols)).toEqual([]);
  });

  test('a full row not ending inside the URL does not swallow the next row', () => {
    const cols = 30;
    const lines = [
      'go to https://example.com now!', // 30 cols, URL ends before "now!"
      '  and then run the tests',
    ];
    const links = findLinksAt(rows(lines), 1, cols);
    expect(links).toEqual([{ text: 'https://example.com', start: { x: 7, y: 1 }, end: { x: 25, y: 1 } }]);
  });

  test('known limitation: URL ending exactly at the last column followed by prose is extended', () => {
    const cols = 26;
    const lines = [
      'link: https://example.com/', // 26 cols: the heuristic cannot tell this from a cut token
      'Then continue reading.',
    ];
    const links = findLinksAt(rows(lines), 1, cols);
    expect(links[0].text).toBe('https://example.com/Then');
  });
});
