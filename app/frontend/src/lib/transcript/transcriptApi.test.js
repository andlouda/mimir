import { afterEach, describe, expect, test, vi } from 'vitest';
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

describe('transcript history', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('reads the width-independent history and its scrubbed variant', async () => {
    const { getTranscriptHistory, getTranscriptHistoryScrubbed } = await import('./transcriptApi.js');
    const plain = vi.fn(async () => ({ resumeId: 'r1', text: 'one long line', size: 13, readBytes: 13, truncated: false }));
    const scrubbed = vi.fn(async () => ({ resumeId: 'r1', text: '[REDACTED]', size: 13, readBytes: 10, truncated: false }));
    vi.stubGlobal('window', { go: { main: { App: { GetTerminalTranscriptHistory: plain, GetTerminalTranscriptHistoryScrubbed: scrubbed } } } });
    expect((await getTranscriptHistory('r1')).text).toBe('one long line');
    expect(plain).toHaveBeenCalledWith('r1', 0);
    expect((await getTranscriptHistoryScrubbed('r1')).text).toBe('[REDACTED]');
  });

  test('a backend without the method or an empty answer yields an empty history', async () => {
    const { getTranscriptHistory, getTranscriptDiskUsage } = await import('./transcriptApi.js');
    vi.stubGlobal('window', { go: { main: { App: {} } } });
    expect(await getTranscriptHistory('r1')).toEqual({ resumeId: 'r1', text: '', size: 0, readBytes: 0, truncated: false });
    vi.stubGlobal('window', { go: { main: { App: { GetTerminalTranscriptHistory: async () => null, GetTranscriptDiskUsage: async () => undefined } } } });
    expect((await getTranscriptHistory('r1')).text).toBe('');
    expect(await getTranscriptDiskUsage()).toEqual({ count: 0, totalBytes: 0 });
  });
});
