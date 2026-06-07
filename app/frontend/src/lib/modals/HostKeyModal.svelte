<script>
  // SSH host-key verification prompt (presentational). Accept/reject are handled
  // by the parent (tied to the connect flow). Styles from the global stylesheets (styles/).
  import { t } from '../i18n.js';

  export let state;            // { status, host, keyType, fingerprint, ... }
  export let onAccept = () => {};
  export let onReject = () => {};
</script>

<div class="modal-overlay" on:click|self={onReject} on:keydown={(e) => { if (e.key === 'Escape') onReject(); }} tabindex="0" role="button">
  <div class="ssh-modal" style="max-width: 480px;">
    <div class="modal-header">
      <h3>{state.status === 'mismatch' ? $t('hostKey.mismatchTitle') : $t('hostKey.unknownTitle')}</h3>
    </div>

    {#if state.status === 'mismatch'}
      <div class="warning-banner" style="background: rgba(244, 112, 103, 0.15); color: #f47067; border-color: rgba(244, 112, 103, 0.3);">
        {$t('hostKey.mismatchWarning')}
      </div>
    {:else}
      <div class="warning-banner" style="background: rgba(227, 179, 65, 0.12); color: #e3b341; border-color: rgba(227, 179, 65, 0.25);">
        {$t('hostKey.unknownWarning')}
      </div>
    {/if}

    <div class="ssh-profile-editor" style="padding: 1rem;">
      <div style="font-size: 0.8rem; color: var(--text-secondary); margin-bottom: 0.75rem;">
        <strong>{$t('hostKey.host')}:</strong> {state.host}
      </div>
      <div style="font-size: 0.8rem; color: var(--text-secondary); margin-bottom: 0.75rem;">
        <strong>{$t('hostKey.keyType')}:</strong> {state.keyType}
      </div>
      <div style="font-size: 0.78rem; font-family: var(--font-mono); color: var(--accent); background: var(--bg-surface); padding: 0.6rem 0.8rem; border-radius: var(--radius-sm); border: 1px solid var(--border-dim); word-break: break-all; margin-bottom: 0.75rem;">
        {state.fingerprint}
      </div>
      <div class="ssh-editor-actions" style="gap: 0.5rem;">
        <button class="header-btn" on:click={onReject}>{$t('hostKey.reject')}</button>
        <button class="add-btn" on:click={onAccept}>
          {state.status === 'mismatch' ? $t('hostKey.acceptMismatch') : $t('hostKey.acceptUnknown')}
        </button>
      </div>
    </div>
  </div>
</div>
