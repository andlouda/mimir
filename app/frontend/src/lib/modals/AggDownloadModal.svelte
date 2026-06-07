<script>
  // Confirmation dialog for downloading the pinned `agg` binary (GIF export).
  // The actual download is handled by the parent. Styles from the global stylesheets (styles/).
  import { t } from '../i18n.js';

  export let info;             // { url, destination, platform }
  export let downloading = false;
  export let onCancel = () => {};
  export let onDownload = () => {};
</script>

<div class="modal-overlay" on:click={onCancel} on:keydown={(e) => { if (e.key === 'Escape') onCancel(); }} tabindex="0" role="button">
  <div class="template-prompt-modal" role="dialog" aria-modal="true" tabindex="-1" on:click|stopPropagation on:keydown|stopPropagation>
    <div class="template-prompt-header">
      <h3>{$t('aggDownload.title')}</h3>
      <button type="button" class="modal-close-button" on:click={onCancel}>&#x2715;</button>
    </div>
    <p class="template-prompt-text">{$t('aggDownload.intro')}</p>
    <div class="agg-download-details">
      <div class="agg-detail-row">
        <span class="agg-detail-label">{$t('aggDownload.source')}</span>
        <a class="agg-detail-value agg-link" href="https://github.com/asciinema/agg/releases" target="_blank" rel="noopener">{info.url}</a>
      </div>
      <div class="agg-detail-row">
        <span class="agg-detail-label">{$t('aggDownload.destination')}</span>
        <span class="agg-detail-value">{info.destination}</span>
      </div>
      <div class="agg-detail-row">
        <span class="agg-detail-label">{$t('aggDownload.platform')}</span>
        <span class="agg-detail-value">{info.platform}</span>
      </div>
    </div>
    <div class="template-prompt-actions">
      <button type="button" class="modal-secondary-button" on:click={onCancel}>{$t('aggDownload.cancel')}</button>
      <button type="button" class="modal-primary-button" disabled={downloading} on:click={onDownload}>
        {downloading ? $t('aggDownload.downloading') : $t('aggDownload.download')}
      </button>
    </div>
  </div>
</div>
