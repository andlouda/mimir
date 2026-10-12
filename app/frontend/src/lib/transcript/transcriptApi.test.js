import { afterEach, describe, expect, test } from 'vitest';
import { getTranscriptContent, listTranscripts } from './transcriptApi.js';

// The adapter resolves the backend through the facade at call time. A
// missing import there surfaced only in the smoke tests ("backend is not
// defined"), so pin the lookup here.
describe('transcriptApi', () => {
  afterEach(() => {
    delete globalThis.window;
  });

  test('returns safe defaults without a backend', async () => {
    expect(await listTranscripts()).toEqual([]);
    expect(await getTranscriptContent('r1')).toMatchObject({ text: '' });
  });

  test('calls the bound App when present', async () => {
    globalThis.window = { go: { main: { App: {
      ListTranscripts: async () => [{ resumeId: 'r1', size: 1, modTime: 't' }],
      GetTerminalTranscriptContent: async (id) => ({ resumeId: id, text: 'body', size: 4, readBytes: 4, truncated: false }),
    } } } };
    expect(await listTranscripts()).toEqual([{ resumeId: 'r1', size: 1, modTime: 't' }]);
    expect((await getTranscriptContent('r1')).text).toBe('body');
  });
});
