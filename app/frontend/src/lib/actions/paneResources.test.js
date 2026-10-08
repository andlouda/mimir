import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { terminals } from '../stores/terminalStore.js';
import { currentPage, paneResourceOverlay } from '../stores/uiStore.js';
import { formatBytes, formatResources, paneResources, samplePaneResources } from './paneResources.js';

describe('pane resources', () => {
  beforeEach(() => {
    globalThis.window = { go: { main: { App: { GetPaneResourcesJSON: vi.fn(async (ids) => JSON.stringify({ panes: Object.fromEntries(JSON.parse(ids).map((id) => [id, { cpu: 12.4, rss: 1.5 * 1024 ** 3, procs: 4 }])) })) } } } };
    terminals.set([{ id: 1, type: 'bash' }, { id: 2, type: 'ssh' }, { id: 3, type: 'zsh', minimized: true }]);
    currentPage.set('terminals');
    paneResourceOverlay.set(true);
  });
  afterEach(() => { paneResources.set({}); delete globalThis.window; });

  test('asks only for local, visible panes', async () => {
    await samplePaneResources();
    expect(window.go.main.App.GetPaneResourcesJSON).toHaveBeenCalledWith('[1]');
    expect(get(paneResources)[1].procs).toBe(4);
  });

  test('clears when the overlay is off or another page is shown', async () => {
    await samplePaneResources();
    paneResourceOverlay.set(false);
    await samplePaneResources();
    expect(get(paneResources)).toEqual({});
    paneResourceOverlay.set(true);
    currentPage.set('settings');
    await samplePaneResources();
    expect(get(paneResources)).toEqual({});
    expect(window.go.main.App.GetPaneResourcesJSON).toHaveBeenCalledTimes(1);
  });

  test('formats cpu and memory compactly', () => {
    expect(formatResources({ cpu: 12.4, rss: 1.5 * 1024 ** 3 })).toBe('12 % · 1.5 GB');
    expect(formatBytes(345 * 1024 ** 2)).toBe('345 MB');
    expect(formatBytes(12.3 * 1024 ** 3)).toBe('12 GB');
  });
});
