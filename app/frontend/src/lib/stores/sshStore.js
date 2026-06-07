import { writable } from 'svelte/store';

export const sshProfiles = writable([]);
export const showSSHProfileModal = writable(false);
export const sshSecretBackend = writable('');
export const sshConnecting = writable(false);
export const hostKeyVerifyState = writable(null);
export const fileBrowserRemoteTerminalId = writable(0);
export const fileBrowserRemoteLabel = writable('');
