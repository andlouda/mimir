import { get } from 'svelte/store';
import { errorMessage } from '../stores/uiStore.js';
import { updateChecking, updateDownloading, updateInfo, updateProgress } from '../stores/updateStore.js';

function app() {
  return window['go']['main']['App'];
}

export async function checkForUpdates() {
  updateChecking.set(true);
  try {
    const raw = await app()['CheckForUpdates']();
    updateInfo.set(JSON.parse(raw));
  } catch (error) {
    updateInfo.set({ error: error.message || String(error), configured: false });
  } finally {
    updateChecking.set(false);
  }
}

export async function openUpdatePage() {
  try {
    await app()['OpenUpdatePage'](get(updateInfo)?.releaseUrl || '');
  } catch (error) {
    errorMessage.set(`Update-Seite konnte nicht geoeffnet werden: ${error.message || error}`);
  }
}

export async function downloadUpdate() {
  updateDownloading.set(true);
  updateProgress.set({ stage: 'downloading', percent: 0 });
  try {
    const raw = await app()['StartUpdateDownload']();
    const result = JSON.parse(raw);
    if (result.error) {
      errorMessage.set(`Update failed: ${result.error}`);
      updateProgress.set(null);
      updateDownloading.set(false);
    }
  } catch (error) {
    errorMessage.set(`Update failed: ${error.message || error}`);
    updateProgress.set(null);
    updateDownloading.set(false);
  }
}

export async function restartApp() {
  try {
    await app()['RestartApp']();
  } catch (error) {
    errorMessage.set(`Restart failed: ${error.message || error}`);
  }
}
