<script>
  import { onDestroy, onMount, tick } from 'svelte';
  import { t } from '../i18n.js';
  import { cleanTranscript } from '../transcript/cleanTranscript.js';
  import { getTranscriptContent, listTranscripts } from '../transcript/transcriptApi.js';

  export let initialResumeId = '';
  export let initialLabel = '';
  export let onClose = () => {};
  export let onError = () => {};

  let modalEl;
  let viewerEl;

  let entries = [];
  let selectedResumeId = initialResumeId || '';
  let transcriptText = '';
  let transcriptSize = 0;
  let transcriptReadBytes = 0;
  let loadingList = true;
  let loadingTranscript = false;
  let listError = '';
  let transcriptError = '';
  let truncated = false;
  let showRaw = false;
  // Start collapsed when we were opened with a known resumeId (the user
  // pressed the button on a specific terminal) so the viewer takes the full
  // width by default. Browse mode is one click away via the header toggle.
  let listOpen = !initialResumeId;

  // Stale-load guard. Bumped on every loadTranscript call; the in-flight
  // promise only commits its result if its token still matches. Without this,
  // switching transcripts faster than the IPC round-trip lets an older
  // response clobber a newer one.
  let loadToken = 0;

  // Memoize the clean-mode result. Svelte's reactivity recomputes whenever
  // ANY dependency of a $: statement changes; without the cache we'd re-run
  // cleanTranscript on every modal-state flip even though neither the input
  // text nor the mode changed.
  let cleanCacheKey = '';
  let cleanCacheValue = '';
  function getDisplayText(text, raw) {
    if (raw) return text;
    if (cleanCacheKey === text) return cleanCacheValue;
    cleanCacheKey = text;
    cleanCacheValue = cleanTranscript(text);
    return cleanCacheValue;
  }
  $: displayText = getDisplayText(transcriptText, showRaw);

  $: selectedEntry = selectedResumeId
    ? entries.find((entry) => entry.resumeId === selectedResumeId)
    : null;

  $: subtitle = selectedEntry
    ? entryLabel(selectedEntry)
    : initialLabel;

  function formatBytes(n) {
    if (!Number.isFinite(n)) return '';
    if (n < 1024) return `${n} B`;
    const units = ['KB', 'MB', 'GB'];
    let value = n / 1024;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
      value /= 1024;
      unit += 1;
    }
    return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unit]}`;
  }

  function formatRelative(iso) {
    if (!iso) return '';
    const ts = new Date(iso).getTime();
    if (!Number.isFinite(ts)) return '';
    const diff = Date.now() - ts;
    const minute = 60_000;
    const hour = 60 * minute;
    const day = 24 * hour;
    if (diff < minute) return $t('transcriptViewer.justNow');
    if (diff < hour) return $t('transcriptViewer.minutesAgo', { n: Math.floor(diff / minute) });
    if (diff < day) return $t('transcriptViewer.hoursAgo', { n: Math.floor(diff / hour) });
    return new Date(ts).toLocaleString();
  }

  function entryLabel(entry) {
    if (entry.name) return entry.name;
    if (entry.sshProfileId) return entry.sshProfileId;
    if (entry.type) return $t('transcriptViewer.unnamedOf', { type: entry.type });
    return $t('transcriptViewer.unnamed');
  }

  async function loadList() {
    loadingList = true;
    listError = '';
    try {
      entries = await listTranscripts();
      if (!selectedResumeId && entries.length > 0) {
        selectedResumeId = entries[0].resumeId;
      }
    } catch (error) {
      const message = error?.message || String(error);
      listError = message;
      onError(`Failed to load transcripts: ${message}`);
      entries = [];
    } finally {
      loadingList = false;
    }
  }

  async function loadTranscript(resumeId) {
    if (!resumeId) {
      transcriptText = '';
      transcriptSize = 0;
      transcriptReadBytes = 0;
      truncated = false;
      transcriptError = '';
      return;
    }
    const token = ++loadToken;
    loadingTranscript = true;
    transcriptError = '';
    try {
      const content = await getTranscriptContent(resumeId, 0);
      if (token !== loadToken) return; // a newer request superseded us
      transcriptText = content.text;
      transcriptSize = content.size;
      transcriptReadBytes = content.readBytes;
      truncated = content.truncated;
      await tick();
      if (viewerEl) viewerEl.scrollTop = viewerEl.scrollHeight;
    } catch (error) {
      if (token !== loadToken) return;
      const message = error?.message || String(error);
      transcriptError = message;
      onError(`Failed to load transcript: ${message}`);
      transcriptText = '';
      transcriptSize = 0;
      transcriptReadBytes = 0;
      truncated = false;
    } finally {
      if (token === loadToken) loadingTranscript = false;
    }
  }

  function retryLoadTranscript() {
    if (selectedResumeId) loadTranscript(selectedResumeId);
  }

  function retryLoadList() {
    loadList();
  }

  function select(resumeId) {
    listOpen = false;
    if (resumeId === selectedResumeId) return;
    selectedResumeId = resumeId;
  }

  async function copyAll() {
    if (!transcriptText) return;
    try {
      // Copy whatever the user actually sees — clean by default, raw if
      // they explicitly toggled it.
      await navigator.clipboard.writeText(displayText);
    } catch (error) {
      onError(`Copy failed: ${error?.message || error}`);
    }
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') onClose();
  }

  // Close on any pointer interaction outside the modal. We listen at the
  // document level on the capture phase because xterm.js installs its own
  // bubbling-phase listeners on the terminal area — a click on the terminal
  // would otherwise be swallowed before reaching our backdrop.
  function handleDocumentPointer(event) {
    if (!modalEl) return;
    if (event.composedPath?.().includes(modalEl)) return;
    if (modalEl.contains(event.target)) return;
    onClose();
  }

  onMount(async () => {
    document.addEventListener('mousedown', handleDocumentPointer, true);
    document.addEventListener('touchstart', handleDocumentPointer, true);
    // The reactive `$: if (selectedResumeId) loadTranscript(...)` below is
    // the single source of truth for the load — calling it explicitly here
    // (and racing with reactivity-after-loadList) used to fire two
    // round-trips on every open. loadList alone is enough; selectedResumeId
    // either was set by the caller (then the reactive fires once already)
    // or gets set inside loadList (then the reactive fires once after).
    await loadList();
  });

  onDestroy(() => {
    document.removeEventListener('mousedown', handleDocumentPointer, true);
    document.removeEventListener('touchstart', handleDocumentPointer, true);
  });

  $: if (selectedResumeId) loadTranscript(selectedResumeId);
</script>

<div
  class="modal-overlay"
  on:click|self={onClose}
  on:keydown={handleKeydown}
  role="presentation"
>
  <div class="transcript-viewer" bind:this={modalEl} role="dialog" aria-modal="true" tabindex="-1">
    <header class="transcript-viewer-header">
      <div>
        <h3>{$t('transcriptViewer.title')}</h3>
        {#if subtitle}
          <p class="transcript-viewer-subtitle">{subtitle}</p>
        {/if}
      </div>
      <div class="transcript-viewer-header-actions">
        <button
          type="button"
          class="transcript-viewer-browse"
          class:active={listOpen}
          on:click={() => { listOpen = !listOpen; }}
          aria-pressed={listOpen}
          title={$t('transcriptViewer.browseLabel')}
        >
          {$t('transcriptViewer.browse', { n: entries.length })}
        </button>
        <button type="button" class="modal-close-button" on:click={onClose} aria-label={$t('transcriptViewer.close')}>
          &#x2715;
        </button>
      </div>
    </header>

    <div class="transcript-viewer-body" class:list-hidden={!listOpen}>
      {#if listOpen}
      <aside class="transcript-viewer-list" aria-label={$t('transcriptViewer.listLabel')}>
        {#if loadingList}
          <div class="transcript-viewer-empty">{$t('transcriptViewer.loadingList')}</div>
        {:else if listError}
          <div class="transcript-viewer-empty transcript-viewer-error">
            <p>{$t('transcriptViewer.listErrorTitle')}</p>
            <button type="button" class="modal-secondary-button" on:click={retryLoadList}>
              {$t('transcriptViewer.retry')}
            </button>
          </div>
        {:else if entries.length === 0}
          <div class="transcript-viewer-empty">{$t('transcriptViewer.empty')}</div>
        {:else}
          <ul>
            {#each entries as entry (entry.resumeId)}
              <li>
                <button
                  type="button"
                  class="transcript-viewer-entry"
                  class:active={entry.resumeId === selectedResumeId}
                  on:click={() => select(entry.resumeId)}
                >
                  <span class="transcript-entry-label">{entryLabel(entry)}</span>
                  <span class="transcript-entry-meta">
                    {formatRelative(entry.modTime)} · {formatBytes(entry.size)}
                  </span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </aside>
      {/if}

      <section class="transcript-viewer-pane">
        {#if loadingTranscript}
          <div class="transcript-viewer-empty">{$t('transcriptViewer.loadingTranscript')}</div>
        {:else if transcriptError}
          <div class="transcript-viewer-empty transcript-viewer-error">
            <p>{$t('transcriptViewer.loadErrorTitle')}</p>
            <button type="button" class="modal-secondary-button" on:click={retryLoadTranscript}>
              {$t('transcriptViewer.retry')}
            </button>
          </div>
        {:else if !selectedResumeId}
          <div class="transcript-viewer-empty">{$t('transcriptViewer.pickOne')}</div>
        {:else if !transcriptText}
          <div class="transcript-viewer-empty">{$t('transcriptViewer.emptyTranscript')}</div>
        {:else}
          {#if truncated}
            <div class="transcript-viewer-truncated">
              {$t('transcriptViewer.truncatedDetail', {
                read: formatBytes(transcriptReadBytes),
                total: formatBytes(transcriptSize),
              })}
            </div>
          {/if}
          <pre bind:this={viewerEl} class="transcript-viewer-content">{displayText}</pre>
        {/if}
      </section>
    </div>

    <footer class="transcript-viewer-footer">
      <label class="transcript-viewer-toggle">
        <input type="checkbox" bind:checked={showRaw} />
        <span>{$t('transcriptViewer.showRaw')}</span>
      </label>
      <div class="transcript-viewer-actions">
        <button
          type="button"
          class="modal-secondary-button"
          on:click={copyAll}
          disabled={!transcriptText}
        >
          {$t('transcriptViewer.copyAll')}
        </button>
        <button type="button" class="modal-primary-button" on:click={onClose}>
          {$t('transcriptViewer.close')}
        </button>
      </div>
    </footer>
  </div>
</div>

<style>
  .transcript-viewer {
    display: flex;
    flex-direction: column;
    width: min(960px, 92vw);
    height: min(720px, 88vh);
    background: #15171f;
    border: 1px solid #2b3140;
    border-radius: 10px;
    box-shadow: 0 24px 64px rgba(0, 0, 0, 0.45);
    color: #d6dae3;
    overflow: hidden;
  }
  .transcript-viewer-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    padding: 14px 18px;
    border-bottom: 1px solid #2b3140;
  }
  .transcript-viewer-header-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .transcript-viewer-browse {
    background: transparent;
    color: #c8cdd8;
    border: 1px solid #2b3140;
    border-radius: 6px;
    padding: 4px 10px;
    font-size: 12px;
    cursor: pointer;
    font: inherit;
    line-height: 1.4;
  }
  .transcript-viewer-browse:hover {
    background: #1a1d27;
  }
  .transcript-viewer-browse.active {
    background: #1c2230;
    border-color: #63b3ed;
    color: #e7ecf3;
  }
  .transcript-viewer-header h3 {
    margin: 0;
    font-size: 14px;
    font-weight: 600;
    color: #e7ecf3;
  }
  .transcript-viewer-subtitle {
    margin: 4px 0 0;
    font-size: 12px;
    color: #8a93a4;
  }
  .transcript-viewer-body {
    display: flex;
    flex: 1;
    min-height: 0;
  }
  .transcript-viewer-list {
    width: 260px;
    border-right: 1px solid #2b3140;
    overflow-y: auto;
    background: #11131a;
  }
  .transcript-viewer-list ul {
    list-style: none;
    margin: 0;
    padding: 6px 0;
  }
  .transcript-viewer-entry {
    width: 100%;
    text-align: left;
    background: transparent;
    border: 0;
    color: inherit;
    padding: 8px 14px;
    cursor: pointer;
    display: flex;
    flex-direction: column;
    gap: 2px;
    border-left: 2px solid transparent;
    font: inherit;
  }
  .transcript-viewer-entry:hover {
    background: #1a1d27;
  }
  .transcript-viewer-entry.active {
    background: #1c2230;
    border-left-color: #63b3ed;
  }
  .transcript-entry-label {
    font-size: 13px;
    color: #dbe1ea;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .transcript-entry-meta {
    font-size: 11px;
    color: #6f7787;
  }
  .transcript-viewer-pane {
    flex: 1;
    display: flex;
    flex-direction: column;
    min-width: 0;
    background: #0c0e14;
  }
  .transcript-viewer-content {
    flex: 1;
    margin: 0;
    padding: 14px 16px;
    overflow: auto;
    background: #0c0e14;
    color: #c8cdd8;
    font-family: 'JetBrains Mono', 'Fira Code', Menlo, Consolas, monospace;
    font-size: 12px;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .transcript-viewer-truncated {
    padding: 8px 16px;
    background: #2c241a;
    color: #f3c87a;
    font-size: 12px;
    border-bottom: 1px solid #3a2f1f;
  }
  .transcript-viewer-empty {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: #6f7787;
    font-size: 13px;
    padding: 24px;
    text-align: center;
  }
  .transcript-viewer-error {
    flex-direction: column;
    gap: 14px;
    color: #f3c87a;
  }
  .transcript-viewer-error p {
    margin: 0;
  }
  .transcript-viewer-footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 12px 18px;
    border-top: 1px solid #2b3140;
    background: #11131a;
  }
  .transcript-viewer-toggle {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
    color: #8a93a4;
    user-select: none;
    cursor: pointer;
  }
  .transcript-viewer-toggle input {
    margin: 0;
    accent-color: #63b3ed;
  }
  .transcript-viewer-actions {
    display: flex;
    gap: 8px;
  }
</style>
