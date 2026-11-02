import { describe, expect, test } from 'vitest';
import { blobToBase64, clipboardHasImage, clipboardImageFile, imagePasteMode, pasteAction } from './clipboardImage.js';

const data = ({ text = '', types = [], items = [], files = [] } = {}) => ({ getData: (t) => (t === 'text' ? text : ''), types, items, files });

describe('clipboard image paste', () => {
  test('text wins over an image', () => {
    expect(pasteAction(data({ text: 'ls', types: ['text/plain', 'image/png'] }))).toEqual({ kind: 'text', text: 'ls' });
  });
  test('an image without text is an image paste, with the file when the browser hands it over', () => {
    expect(pasteAction(data({ types: ['image/png'] }))).toEqual({ kind: 'image', file: null });
    const jpeg = { type: 'image/jpeg' };
    expect(pasteAction(data({ items: [{ kind: 'file', type: 'image/jpeg', getAsFile: () => jpeg }] }))).toEqual({ kind: 'image', file: jpeg });
    const webp = { type: 'image/webp' };
    expect(pasteAction(data({ files: [webp] }))).toEqual({ kind: 'image', file: webp });
  });
  test('the first image file wins over non-image items', () => {
    const png = { type: 'image/png' };
    expect(clipboardImageFile(data({ items: [{ kind: 'string', type: 'text/html' }, { kind: 'file', type: 'image/png', getAsFile: () => png }] }))).toBe(png);
  });
  test('SSH panes get the image as a file, local panes follow the setting', () => {
    expect(imagePasteMode({ type: 'ssh' })).toBe('file');
    expect(imagePasteMode({ type: 'ssh' }, 'clipboard')).toBe('file');
    for (const type of ['bash', 'zsh', 'wsl', 'powershell', 'cmd']) {
      expect(imagePasteMode({ type })).toBe('keystroke');
      expect(imagePasteMode({ type }, 'file')).toBe('file');
    }
  });
  test('blobToBase64 encodes large blobs in chunks', async () => {
    const bytes = new Uint8Array(70000).map((_, i) => i % 251);
    const blob = { arrayBuffer: async () => bytes.buffer };
    expect(atob(await blobToBase64(blob)).length).toBe(70000);
  });
  test('an empty clipboard does nothing', () => {
    expect(pasteAction(data())).toEqual({ kind: 'none' });
    expect(pasteAction(null)).toEqual({ kind: 'none' });
    expect(clipboardHasImage(data({ items: [{ kind: 'string', type: 'text/html' }] }))).toBe(false);
  });
});
