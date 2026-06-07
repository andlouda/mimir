import { afterEach, describe, expect, test } from 'vitest';
import { get } from 'svelte/store';
import { updateChecking, updateDownloading, updateInfo, updateInstalled, updateProgress } from './updateStore.js';

afterEach(() => {
  updateInfo.set(null);
  updateChecking.set(false);
  updateDownloading.set(false);
  updateProgress.set(null);
  updateInstalled.set(false);
});

describe('update store', () => {
  test('exposes default update state', () => {
    expect(get(updateInfo)).toBe(null);
    expect(get(updateChecking)).toBe(false);
    expect(get(updateDownloading)).toBe(false);
    expect(get(updateProgress)).toBe(null);
    expect(get(updateInstalled)).toBe(false);
  });

  test('tracks update progress and installed marker', () => {
    updateInfo.set({ currentVersion: '1.0.0', latestVersion: '1.1.0', updateAvailable: true });
    updateDownloading.set(true);
    updateProgress.set({ stage: 'downloading', percent: 42 });
    updateInstalled.set(true);

    expect(get(updateInfo).latestVersion).toBe('1.1.0');
    expect(get(updateDownloading)).toBe(true);
    expect(get(updateProgress)).toEqual({ stage: 'downloading', percent: 42 });
    expect(get(updateInstalled)).toBe(true);
  });
});
