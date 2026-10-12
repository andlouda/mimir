import { get } from 'svelte/store';
import { backend as app, DownloadAgg, GetAggDownloadInfo, IsAggInstalled } from '../backend.js';
import { t } from '../i18n.js';
import { aggAvailable, aggDownloadInfo, aggStatus, downloadingAgg } from '../stores/sessionStore.js';
import { errorMessage } from '../stores/uiStore.js';

// Fetches what the install dialog shows (release asset, checksum); a no-op
// while agg is already installed.
export async function loadAggDownloadInfo() {
  if (get(aggAvailable)) return;
  try {
    aggDownloadInfo.set(await GetAggDownloadInfo());
  } catch (err) {
    errorMessage.set(get(t)('appMessages.aggInfoFailed', { error: err.message || err }));
  }
}

export async function runAggDownload() {
  downloadingAgg.set(true);
  try {
    await DownloadAgg();
    const installed = await IsAggInstalled().catch(() => false);
    aggAvailable.set(installed);
    const status = await app()['GetAggStatus']().catch(() => 'missing');
    aggStatus.set(status);
    aggDownloadInfo.set(null);
    if (!installed && status === 'incompatible') {
      errorMessage.set('agg was downloaded but is incompatible with your system. Install agg via your package manager (e.g. cargo install agg).');
    }
  } catch (error) {
    errorMessage.set(`agg Download fehlgeschlagen: ${error.message || error}`);
  } finally {
    downloadingAgg.set(false);
  }
}
