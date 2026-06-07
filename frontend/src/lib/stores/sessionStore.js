import { writable } from 'svelte/store';

export const recordingList = writable([]);
export const aggAvailable = writable(false);
export const aggStatus = writable('missing');
export const downloadingAgg = writable(false);
export const aggDownloadInfo = writable(null);
export const terminalSessionFoldersOpen = writable({ local: true, ssh: true, windows: true, other: true });
export const customFolders = writable([]);
export const newFolderName = writable('');
