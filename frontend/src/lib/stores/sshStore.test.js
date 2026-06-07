import { afterEach, describe, expect, test } from 'vitest';
import { get } from 'svelte/store';
import {
  fileBrowserRemoteLabel,
  fileBrowserRemoteTerminalId,
  hostKeyVerifyState,
  showSSHProfileModal,
  sshConnecting,
  sshProfiles,
  sshSecretBackend,
} from './sshStore.js';

afterEach(() => {
  sshProfiles.set([]);
  showSSHProfileModal.set(false);
  sshSecretBackend.set('');
  sshConnecting.set(false);
  hostKeyVerifyState.set(null);
  fileBrowserRemoteTerminalId.set(0);
  fileBrowserRemoteLabel.set('');
});

describe('ssh store', () => {
  test('exposes default SSH state', () => {
    expect(get(sshProfiles)).toEqual([]);
    expect(get(showSSHProfileModal)).toBe(false);
    expect(get(sshSecretBackend)).toBe('');
    expect(get(sshConnecting)).toBe(false);
    expect(get(hostKeyVerifyState)).toBe(null);
  });

  test('tracks profile, host key and remote browser state', () => {
    sshProfiles.set([{ id: 'prod', name: 'Prod' }]);
    showSSHProfileModal.set(true);
    sshSecretBackend.set('keyring');
    sshConnecting.set(true);
    hostKeyVerifyState.set({ status: 'unknown', host: 'example.test', profileID: 'prod' });
    fileBrowserRemoteTerminalId.set(7);
    fileBrowserRemoteLabel.set('ops@example.test');

    expect(get(sshProfiles)[0].id).toBe('prod');
    expect(get(showSSHProfileModal)).toBe(true);
    expect(get(sshSecretBackend)).toBe('keyring');
    expect(get(sshConnecting)).toBe(true);
    expect(get(hostKeyVerifyState).host).toBe('example.test');
    expect(get(fileBrowserRemoteTerminalId)).toBe(7);
    expect(get(fileBrowserRemoteLabel)).toBe('ops@example.test');
  });
});
