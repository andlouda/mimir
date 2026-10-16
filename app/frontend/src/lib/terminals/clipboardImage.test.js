import { describe, expect, test } from 'vitest';
import { clipboardHasImage, pasteAction } from './clipboardImage.js';

const data = ({ text = '', types = [], items = [], files = [] } = {}) => ({ getData: (t) => (t === 'text' ? text : ''), types, items, files });

describe('clipboard image paste', () => {
  test('text wins over an image', () => {
    expect(pasteAction(data({ text: 'ls', types: ['text/plain', 'image/png'] }))).toEqual({ kind: 'text', text: 'ls' });
  });
  test('an image without text becomes a Ctrl+V for the program', () => {
    expect(pasteAction(data({ types: ['image/png'] }))).toEqual({ kind: 'image' });
    expect(pasteAction(data({ items: [{ kind: 'file', type: 'image/jpeg' }] }))).toEqual({ kind: 'image' });
    expect(pasteAction(data({ files: [{ type: 'image/webp' }] }))).toEqual({ kind: 'image' });
  });
  test('an empty clipboard does nothing', () => {
    expect(pasteAction(data())).toEqual({ kind: 'none' });
    expect(pasteAction(null)).toEqual({ kind: 'none' });
    expect(clipboardHasImage(data({ items: [{ kind: 'string', type: 'text/html' }] }))).toBe(false);
  });
});
