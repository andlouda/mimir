import { writable } from 'svelte/store';

export const updateInfo = writable(null);
export const updateChecking = writable(false);
export const updateDownloading = writable(false);
export const updateProgress = writable(null);
export const updateInstalled = writable(false);
