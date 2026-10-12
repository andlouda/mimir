// @vitest-environment jsdom
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import { get } from 'svelte/store';
import AppModals from './AppModals.svelte';
import { aggDownloadInfo } from './stores/sessionStore.js';
import { hostKeyVerifyState } from './stores/sshStore.js';
import { showTemplatePicker, templates } from './stores/templateStore.js';
import { activeTerminalId, terminals } from './stores/terminalStore.js';
import { errorMessage, transcriptViewerState } from './stores/uiStore.js';
import { installBackendStub, removeBackendStub } from '../test/backendStub.js';

// Every modal is driven by a store; AppModals shows it while the store is
// set and routes its buttons to the actions.

let view;
let stub;

beforeEach(() => {
  stub = installBackendStub({
    ListTranscripts: async () => [],
  });
  showTemplatePicker.set(false);
  templates.set([]);
  terminals.set([]);
  activeTerminalId.set(null);
  hostKeyVerifyState.set(null);
  aggDownloadInfo.set(null);
  transcriptViewerState.set(null);
  errorMessage.set('');
  view = mount(AppModals, { target: document.body });
  flushSync();
});

afterEach(() => {
  unmount(view);
  document.body.innerHTML = '';
  removeBackendStub();
});

test('renders nothing while no modal store is set', () => {
  expect(document.querySelector('.modal-overlay')).toBeNull();
});

test('template picker lists the templates and closes on choice', () => {
  templates.set([{ name: 'Deploy', description: 'ship it', category: 'ops', command: 'make deploy', favorite: false }]);
  showTemplatePicker.set(true);
  flushSync();
  const item = document.querySelector('.template-picker-item');
  expect(item?.textContent).toContain('Deploy');
  item.click();
  flushSync();
  expect(get(showTemplatePicker)).toBe(false);
  // applyTemplate ran: without an active terminal it refuses.
  expect(get(errorMessage)).toBe('Please select a terminal first.');
});

test('host key prompt rejects through the backend and clears its state', async () => {
  hostKeyVerifyState.set({ status: 'unknown', host: 'db.example', keyType: 'ed25519', fingerprint: 'SHA256:abc', message: '', profileID: 'p1', profile: { id: 'p1', name: 'db' } });
  flushSync();
  expect(document.body.textContent).toContain('Unknown Host');
  [...document.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Reject').click();
  await vi.waitFor(() => expect(get(hostKeyVerifyState)).toBeNull());
  expect(stub.calls).toContainEqual(['RejectSSHHostKey', 'p1']);
});

test('agg download dialog cancels back into the store', () => {
  aggDownloadInfo.set({ url: 'https://example/agg', destination: '/tmp/agg', platform: 'linux-x64' });
  flushSync();
  expect(document.body.textContent).toContain('Install agg');
  [...document.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Cancel').click();
  flushSync();
  expect(get(aggDownloadInfo)).toBeNull();
  expect(document.querySelector('.modal-overlay')).toBeNull();
});

test('transcript viewer opens for the stored resume id and lists transcripts', async () => {
  transcriptViewerState.set({ resumeId: 'r1', label: 'BASH 1' });
  flushSync();
  await vi.waitFor(() => expect(stub.calls.some(([name]) => name === 'ListTranscripts' || name === 'GetTerminalTranscriptContent')).toBe(true));
  expect(document.querySelector('[role="dialog"], .modal-overlay')).not.toBeNull();
});
