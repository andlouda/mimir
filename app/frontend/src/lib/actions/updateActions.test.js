import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { errorMessage } from '../stores/uiStore.js';
import { updateChecking, updateDownloading, updateInfo, updateProgress } from '../stores/updateStore.js';
import { checkForUpdates, downloadUpdate, openUpdatePage, restartApp } from './updateActions.js';

beforeEach(() => {
  globalThis.window = {
    go: {
      main: {
        App: {
          CheckForUpdates: vi.fn(),
          OpenUpdatePage: vi.fn(),
          StartUpdateDownload: vi.fn(),
          RestartApp: vi.fn(),
        },
      },
    },
  };
});

afterEach(() => {
  updateInfo.set(null);
  updateChecking.set(false);
  updateDownloading.set(false);
  updateProgress.set(null);
  errorMessage.set('');
  vi.restoreAllMocks();
  delete globalThis.window;
});

describe('update actions', () => {
  test('loads update info from the backend', async () => {
    window.go.main.App.CheckForUpdates.mockResolvedValue(JSON.stringify({ updateAvailable: true, latestVersion: '1.2.3' }));

    await checkForUpdates();

    expect(get(updateChecking)).toBe(false);
    expect(get(updateInfo)).toMatchObject({ updateAvailable: true, latestVersion: '1.2.3' });
  });

  test('opens the release page from current update info', async () => {
    updateInfo.set({ releaseUrl: 'https://example.invalid/release' });

    await openUpdatePage();

    expect(window.go.main.App.OpenUpdatePage).toHaveBeenCalledWith('https://example.invalid/release');
  });

  test('records download errors and clears progress state', async () => {
    window.go.main.App.StartUpdateDownload.mockResolvedValue(JSON.stringify({ error: 'checksum mismatch' }));

    await downloadUpdate();

    expect(get(errorMessage)).toBe('Update failed: checksum mismatch');
    expect(get(updateProgress)).toBe(null);
    expect(get(updateDownloading)).toBe(false);
  });

  test('surfaces restart errors', async () => {
    window.go.main.App.RestartApp.mockRejectedValue(new Error('not supported'));

    await restartApp();

    expect(get(errorMessage)).toBe('Restart failed: not supported');
  });
});
