import { afterEach, describe, expect, test } from 'vitest';
import { get } from 'svelte/store';
import {
  aggAvailable,
  aggDownloadInfo,
  aggStatus,
  customFolders,
  downloadingAgg,
  newFolderName,
  recordingList,
  terminalSessionFoldersOpen,
} from './sessionStore.js';

afterEach(() => {
  recordingList.set([]);
  aggAvailable.set(false);
  aggStatus.set('missing');
  downloadingAgg.set(false);
  aggDownloadInfo.set(null);
  terminalSessionFoldersOpen.set({ local: true, ssh: true, windows: true, other: true });
  customFolders.set([]);
  newFolderName.set('');
});

describe('session store', () => {
  test('exposes default session state', () => {
    expect(get(recordingList)).toEqual([]);
    expect(get(aggAvailable)).toBe(false);
    expect(get(aggStatus)).toBe('missing');
    expect(get(downloadingAgg)).toBe(false);
    expect(get(aggDownloadInfo)).toBe(null);
    expect(get(terminalSessionFoldersOpen)).toEqual({ local: true, ssh: true, windows: true, other: true });
  });

  test('tracks folders, recordings and agg download state', () => {
    customFolders.set([{ id: 'ops', name: 'Ops', position: 1 }]);
    newFolderName.set('Scratch');
    recordingList.set([{ id: 'rec-1', title: 'Bash' }]);
    aggAvailable.set(true);
    downloadingAgg.set(true);
    aggDownloadInfo.set({ url: 'https://example.invalid/agg' });

    expect(get(customFolders)[0].name).toBe('Ops');
    expect(get(newFolderName)).toBe('Scratch');
    expect(get(recordingList)).toHaveLength(1);
    expect(get(aggAvailable)).toBe(true);
    expect(get(downloadingAgg)).toBe(true);
    expect(get(aggDownloadInfo).url).toContain('agg');
  });
});
