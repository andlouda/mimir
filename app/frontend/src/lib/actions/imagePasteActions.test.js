import { get } from 'svelte/store';
import { beforeEach, describe, expect, test } from 'vitest';
import { errorMessage } from '../stores/uiStore.js';
import { pasteImageAsFile } from './imagePasteActions.js';

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
