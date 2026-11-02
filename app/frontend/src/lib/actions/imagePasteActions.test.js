import { get } from 'svelte/store';
import { beforeEach, describe, expect, test } from 'vitest';
import { errorMessage } from '../stores/uiStore.js';
import { pasteImageAsFile, readClipboardImage } from './imagePasteActions.js';

const file = (type = 'image/png', bytes = [137, 80, 78, 71]) => ({ type, arrayBuffer: async () => Uint8Array.from(bytes).buffer });
const term = (type = 'ssh') => { const pasted = []; return { id: 7, type, terminal: { paste: (s) => pasted.push(s) }, pasted }; };

describe('pasteImageAsFile', () => {
  beforeEach(() => errorMessage.set(''));

  test('uploads the image and pastes the quoted remote path', async () => {
    const calls = [];
    const t = term();
    const save = async (...args) => { calls.push(args); return "/home/me/.cache/mimir/paste/paste-1.png"; };
    expect(await pasteImageAsFile(t, file(), { save })).toBe('/home/me/.cache/mimir/paste/paste-1.png');
    expect(calls).toEqual([[7, 'image/png', btoa(String.fromCharCode(137, 80, 78, 71))]]);
    expect(t.pasted).toEqual(['/home/me/.cache/mimir/paste/paste-1.png']);
    expect(get(errorMessage)).toBe('');
  });

  test('quotes a path with spaces for the POSIX shell on the host', async () => {
    const t = term();
    await pasteImageAsFile(t, file(), { save: async () => '/home/my user/p 1.png' });
    expect(t.pasted).toEqual(["'/home/my user/p 1.png'"]);
  });

  test('reports a failed upload and pastes nothing', async () => {
    const t = term();
    expect(await pasteImageAsFile(t, file(), { save: async () => { throw new Error('SFTP client failed'); } })).toBe('');
    expect(t.pasted).toEqual([]);
    expect(get(errorMessage)).toContain('SFTP client failed');
  });
});

test('without a File the clipboard API supplies the image', async () => {
  const t = term();
  const png = { type: 'image/png', arrayBuffer: async () => Uint8Array.from([1, 2, 3]).buffer };
  const calls = [];
  const save = async (...args) => { calls.push(args); return '/home/me/.cache/mimir/paste/p.png'; };
  expect(await pasteImageAsFile(t, null, { save, readClipboard: async () => png })).toBe('/home/me/.cache/mimir/paste/p.png');
  expect(calls[0][1]).toBe('image/png');
  expect(t.pasted).toEqual(['/home/me/.cache/mimir/paste/p.png']);
});

test('without any readable image it reports the offered formats and pastes nothing', async () => {
  const t = term();
  const save = async () => { throw new Error('must not be called'); };
  expect(await pasteImageAsFile(t, null, { save, readClipboard: async () => null, types: ['Files', 'text/html'] })).toBe('');
  expect(t.pasted).toEqual([]);
  expect(get(errorMessage)).toContain('Files, text/html');
});

test('readClipboardImage picks the first image item and survives a denied API', async () => {
  const blob = { type: 'image/png' };
  const clipboard = { read: async () => [{ types: ['text/plain'], getType: async () => null }, { types: ['image/png'], getType: async (tp) => (tp === 'image/png' ? blob : null) }] };
  expect(await readClipboardImage({ clipboard })).toBe(blob);
  expect(await readClipboardImage({ clipboard: { read: async () => { throw new Error('denied'); } } })).toBeNull();
  expect(await readClipboardImage({ clipboard: undefined })).toBeNull();
});
