import { backend as app, DownloadAgg, IsAggInstalled } from '../backend.js';
import { aggAvailable, aggDownloadInfo, aggStatus, downloadingAgg } from '../stores/sessionStore.js';
import { errorMessage } from '../stores/uiStore.js';

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
