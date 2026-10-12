// @vitest-environment jsdom
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import { get } from 'svelte/store';
import SettingsView from './SettingsView.svelte';
import { aggAvailable, aggDownloadInfo, customFolders, newFolderName } from '../stores/sessionStore.js';
import { currentPage, historyTrackingEnabled, notesPanelOpen, showFolderManager } from '../stores/uiStore.js';
import { updateInfo } from '../stores/updateStore.js';
import { installBackendStub, removeBackendStub } from '../../test/backendStub.js';

// The settings view reads stores and calls actions directly (no props), so
// a test sets the stores, clicks a card button and checks the store and the
// backend call that follow.

let view;
let stub;

function cardButton(title) {
  const card = [...document.querySelectorAll('.ai-hub-card')].find((c) => c.querySelector('.settings-card-title strong')?.textContent.trim() === title);
  if (!card) throw new Error(`no card titled ${title}`);
  return card.querySelector('.ai-hub-link');
}

beforeEach(() => {
  stub = installBackendStub({
    CheckForUpdates: async () => JSON.stringify({ configured: true, currentVersion: '0.8.0', latestVersion: '0.8.1', updateAvailable: true }),
    GetAggDownloadInfo: async () => ({ assetName: 'agg-x86_64', version: '1.5.0' }),
    SaveTerminalFolder: async (json) => [{ id: 'f1', name: JSON.parse(json).name, position: 1 }],
  });
  showFolderManager.set(false);
  historyTrackingEnabled.set(false);
  notesPanelOpen.set(false);
  currentPage.set('settings');
  aggAvailable.set(false);
  aggDownloadInfo.set(null);
  customFolders.set([]);
  newFolderName.set('');
  updateInfo.set(null);
  view = mount(SettingsView, { target: document.body });
  flushSync();
});

afterEach(() => {
  unmount(view);
  document.body.innerHTML = '';
  removeBackendStub();
});

test('folder manager opens from its card and creates a folder through the backend', async () => {
  expect(cardButton('Terminal Folders').textContent.trim()).toBe('Manage');
  cardButton('Terminal Folders').click();
  flushSync();
  expect(get(showFolderManager)).toBe(true);
  expect(cardButton('Terminal Folders').textContent.trim()).toBe('Close');

  const input = document.querySelector('.folder-manager input, input[placeholder]');
  expect(input).toBeTruthy();
  input.value = 'Ops';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  expect(get(newFolderName)).toBe('Ops');

  document.querySelector('.folder-manager-add-btn').click();
  await vi.waitFor(() => expect(get(customFolders)).toEqual([{ id: 'f1', name: 'Ops', position: 1 }]));
  expect(get(newFolderName)).toBe('');
});

test('history card flips the consent through the backend', async () => {
  expect(cardButton('Command History').textContent.trim()).toBe('Disabled');
  cardButton('Command History').click();
  await vi.waitFor(() => expect(get(historyTrackingEnabled)).toBe(true));
  flushSync();
  expect(stub.calls).toContainEqual(['SetHistoryTracking', true]);
  expect(cardButton('Command History').textContent.trim()).toBe('Enabled');
});

test('updates card asks the backend and shows the result', async () => {
  cardButton('Updates').click();
  await vi.waitFor(() => expect(get(updateInfo)?.latestVersion).toBe('0.8.1'));
});

test('agg card loads the download info when agg is missing', async () => {
  expect(cardButton('GIF Export (agg)').textContent.trim()).toBe('Install');
  cardButton('GIF Export (agg)').click();
  await vi.waitFor(() => expect(get(aggDownloadInfo)).toEqual({ assetName: 'agg-x86_64', version: '1.5.0' }));
});

test('agg card is inert once agg is installed', () => {
  aggAvailable.set(true);
  flushSync();
  const button = cardButton('GIF Export (agg)');
  expect(button.disabled).toBe(true);
  expect(button.textContent.trim()).toBe('Installed');
});

test('notes card opens the panel and returns to the terminals page', async () => {
  expect(cardButton('Notes').textContent.trim()).toBe('Open');
  cardButton('Notes').click();
  await vi.waitFor(() => expect(get(notesPanelOpen)).toBe(true));
  expect(get(currentPage)).toBe('terminals');
});
