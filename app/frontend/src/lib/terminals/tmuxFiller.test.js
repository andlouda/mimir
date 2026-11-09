import { describe, expect, test } from 'vitest';
import { cleanTerminalSelection, stripTmuxFiller } from './tmuxFiller.js';
import { findLinksAt } from './multiRowLinks.js';
import { joinSelectionLines } from '../util.js';

// Pane 269 columns, tmux window 230: tmux pads each row to the window width,
// draws │ and fills the rest with dots (the shape reported from an SSH pane).
const COLS = 269;
const WINDOW = 230;
const withFiller = (text) => text.padEnd(WINDOW, ' ') + '│' + '·'.repeat(COLS - WINDOW - 1);
const URL =
  'https://login.example.com/realms/infra/protocol/openid-connect/auth?access_type=offline&client_id=' +
  'kubernetes&code_challenge=AbCdEfGhIjKlMnOpQrStUvWxYz0123456789abcdefg&code_challenge_method=S256' +
  '&nonce=N0nceN0nceN0nceN0nceN0nceN0nceN0nce12345&redirect_uri=urn%3Aietf%3Awg%3Aoauth%3A2.0%3Aoob' +
  '&response_type=code&scope=openid&state=St4teSt4teSt4teSt4te';

function foldedRows(prefix, url, width) {
  const text = prefix + url;
  const out = [];
  for (let i = 0; i < text.length; i += width) out.push(text.slice(i, i + width));
  return out;
}

const rowsOf = (lines) => (y) => (y >= 1 && y <= lines.length ? lines[y - 1].replace(/\s+$/, '') : null);

describe('stripTmuxFiller', () => {
  test('cuts the border and dots and reports the window width', () => {
    expect(stripTmuxFiller(withFiller('$ ls'), COLS)).toEqual({ text: '$ ls', width: WINDOW, filler: true });
  });

  test('rows below the window are pure filler', () => {
    expect(stripTmuxFiller('─'.repeat(WINDOW) + '┘' + '·'.repeat(38), COLS).text).toBe('');
    expect(stripTmuxFiller('·'.repeat(COLS), COLS).text).toBe('');
  });

  test('ordinary rows pass through', () => {
    expect(stripTmuxFiller('a │ b   ', 80)).toEqual({ text: 'a │ b', width: 80, filler: false });
    expect(stripTmuxFiller('────────', 80).filler).toBe(false);
  });
});

describe('cleanTerminalSelection', () => {
  test('removes tmux filler from every copied row', () => {
    const text = ['line one', 'line two', ''].map(withFiller).join('\n');
    expect(cleanTerminalSelection(text)).toBe('line one\nline two\n');
  });

  test('leaves selections without filler untouched', () => {
    expect(cleanTerminalSelection('a  \nb')).toBe('a  \nb');
  });
});

describe('links in a tmux window narrower than the pane', () => {
  test('URL hard-wrapped at 80 columns with filler is one link', () => {
    const prefix = 'Please visit the following URL in your browser: ';
    const folded = foldedRows(prefix, URL, 80);
    expect(folded.length).toBeGreaterThan(3);
    const lines = ['$ kubectl get pods', ...folded.map(withFiller), withFiller('$')];
    const r = rowsOf(lines);
    const last = 1 + folded.length;
    for (let y = 2; y <= last; y++) {
      const links = findLinksAt(r, y, COLS);
      expect(links, `row ${y}`).toHaveLength(1);
      expect(links[0].text).toBe(URL);
      expect(links[0].start).toEqual({ x: prefix.length + 1, y: 2 });
      expect(links[0].end).toEqual({ x: folded[folded.length - 1].length, y: last });
    }
  });

  test('URL wrapped by tmux at the window width (soft wrap drawn as rows) is one link', () => {
    const folded = foldedRows('', URL, WINDOW);
    const lines = folded.map(withFiller);
    const links = findLinksAt(rowsOf(lines), 2, COLS);
    expect(links).toHaveLength(1);
    expect(links[0].text).toBe(URL);
  });

  test('a short URL followed by a word on the next row is not extended', () => {
    const lines = ['see https://example.com', 'done'].map(withFiller);
    const links = findLinksAt(rowsOf(lines), 1, COLS);
    expect(links).toEqual([{ text: 'https://example.com', start: { x: 5, y: 1 }, end: { x: 23, y: 1 } }]);
  });

  test('"Copy as one line" restores the URL from a filler selection', () => {
    const prefix = 'Please visit the following URL in your browser: ';
    const folded = foldedRows(prefix, URL, 80);
    // The user selects from the start of the URL to its end.
    const selected = folded.map(withFiller);
    selected[0] = folded[0].slice(prefix.length).padEnd(WINDOW - prefix.length, ' ') + '│' + '·'.repeat(COLS - WINDOW - 1);
    expect(joinSelectionLines(selected.join('\n'), COLS)).toBe(URL);
    // Including the prose in front keeps the space before the URL.
    expect(joinSelectionLines(folded.map(withFiller).join('\n'), COLS)).toBe(prefix + URL);
  });

  test('"Copy as one line" still spaces ordinary prose', () => {
    const lines = ['first line of prose', 'second line', 'third'].map(withFiller);
    expect(joinSelectionLines(lines.join('\n'), COLS)).toBe('first line of prose second line third');
  });
});
