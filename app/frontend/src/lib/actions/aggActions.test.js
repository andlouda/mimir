import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { DownloadAgg, IsAggInstalled } from '../../../wailsjs/go/main/App';
import { aggAvailable, aggDownloadInfo, aggStatus, downloadingAgg } from '../stores/sessionStore.js';
import { errorMessage } from '../stores/uiStore.js';
import { runAggDownload } from './aggActions.js';

vi.mock('../../../wailsjs/go/main/App', () => ({
  DownloadAgg: vi.fn(),
  IsAggInstalled: vi.fn(),
}));

beforeEach(() => {
  globalThis.window = {
    go: {
      main: {
        App: {
          GetAggStatus: vi.fn(),
        },
      },
    },
  };
});

afterEach(() => {
  aggAvailable.set(false);
  aggStatus.set('missing');
  downloadingAgg.set(false);
  aggDownloadInfo.set(null);
  errorMessage.set('');
  vi.clearAllMocks();
  delete globalThis.window;
});

describe('agg actions', () => {
  test('downloads agg and refreshes installed status', async () => {
    DownloadAgg.mockResolvedValue(undefined);
    IsAggInstalled.mockResolvedValue(true);
    window.go.main.App.GetAggStatus.mockResolvedValue('installed');
    aggDownloadInfo.set({ url: 'https://example.invalid/agg' });

    await runAggDownload();

    expect(DownloadAgg).toHaveBeenCalled();
    expect(get(aggAvailable)).toBe(true);
    expect(get(aggStatus)).toBe('installed');
    expect(get(aggDownloadInfo)).toBe(null);
    expect(get(downloadingAgg)).toBe(false);
  });

  test('surfaces incompatible downloads', async () => {
    DownloadAgg.mockResolvedValue(undefined);
    IsAggInstalled.mockResolvedValue(false);
    window.go.main.App.GetAggStatus.mockResolvedValue('incompatible');

    await runAggDownload();

    expect(get(errorMessage)).toContain('incompatible');
    expect(get(downloadingAgg)).toBe(false);
  });

  test('surfaces download failures', async () => {
    DownloadAgg.mockRejectedValue(new Error('network'));

    await runAggDownload();

    expect(get(errorMessage)).toBe('agg Download fehlgeschlagen: network');
    expect(get(downloadingAgg)).toBe(false);
  });
});
