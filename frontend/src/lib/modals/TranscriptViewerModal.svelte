<script>
  import { onMount, tick } from 'svelte';
  import { t } from '../i18n.js';

  export let initialResumeId = '';
  export let initialLabel = '';
  export let onClose = () => {};
  export let onError = () => {};

  let entries = [];
  let selectedResumeId = initialResumeId || '';
  let transcriptText = '';
  let loadingList = true;
  let loadingTranscript = false;
  let truncated = false;
  let viewerEl;
  let showRaw = false;

  // Strip terminal control sequences so the viewer shows readable text
  // instead of the raw bytes the shell emits (ANSI colors, cursor moves,
  // OSC title pushes, etc.). The raw view is still available via the
  // toggle and the on-disk file is never modified.
  const CSI_RE = /\x1B\[[\d;?<>]*[a-zA-Z]/g;
  const OSC_RE = /\x1B\][^\x07\x1B]*(?:\x07|\x1B\\)/g;
  const ESC_RE = /\x1B[=>()][AB012]?/g;
  const CTRL_RE = /[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]/g;

  function stripAnsi(text) {
    if (!text) return '';
    return text
      .replace(CSI_RE, '')
      .replace(OSC_RE, '')
      .replace(ESC_RE, '')
      .replace(CTRL_RE, '');
  }

  function collapseRepeats(text) {
    // Shells often redraw the same prompt many times in a row (every
    // resize, every cursor blink). Squash runs of identical adjacent
    // lines so the user sees what actually happened.
    const lines = text.split('\n');
    const out = [];
    let prev = null;
    let runCount = 0;
    for (const line of lines) {
      if (line === prev) {
        runCount += 1;
        continue;
      }
      if (runCount > 1) {
        out.push(`  ⟨${runCount - 1}× repeated⟩`);
      }
      out.push(line);
      prev = line;
      runCount = 1;
    }
    if (runCount > 1) out.push(`  ⟨${runCount - 1}× repeated⟩`);
    return out.join('\n');
  }

  $: displayText = showRaw ? transcriptText : collapseRepeats(stripAnsi(transcriptText));

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
    try {
      const list = await window['go']['main']['App']['ListTranscripts']();
      entries = Array.isArray(list) ? list : [];
      if (!selectedResumeId && entries.length > 0) {
        selectedResumeId = entries[0].resumeId;
      }
    } catch (error) {
      onError(`Failed to load transcripts: ${error?.message || error}`);
      entries = [];
    } finally {
      loadingList = false;
    }
  }

  async function loadTranscript(resumeId) {
    if (!resumeId) {
      transcriptText = '';
      truncated = false;
      return;
    }
    loadingTranscript = true;
    try {
      const text = await window['go']['main']['App']['GetTerminalTranscriptFull'](resumeId, 0);
      transcriptText = text || '';
      const entry = entries.find((e) => e.resumeId === resumeId);
      truncated = Boolean(entry && entry.size > transcriptText.length);
      await tick();
      if (viewerEl) viewerEl.scrollTop = viewerEl.scrollHeight;
    } catch (error) {
      onError(`Failed to load transcript: ${error?.message || error}`);
      transcriptText = '';
      truncated = false;
    } finally {
      loadingTranscript = false;
    }
  }

  function select(resumeId) {
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

  onMount(async () => {
    await loadList();
    if (selectedResumeId) await loadTranscript(selectedResumeId);
  });

  $: if (selectedResumeId) loadTranscript(selectedResumeId);
</script>

<div
  class="modal-overlay"
  on:click|self={onClose}
  on:keydown={handleKeydown}
  role="presentation"
>
  <div class="transcript-viewer" role="dialog" aria-modal="true" tabindex="-1">
    <header class="transcript-viewer-header">
      <div>
        <h3>{$t('transcriptViewer.title')}</h3>
        {#if initialLabel}
          <p class="transcript-viewer-subtitle">{initialLabel}</p>
        {/if}
      </div>
      <button type="button" class="modal-close-button" on:click={onClose} aria-label={$t('transcriptViewer.close')}>
        &#x2715;
      </button>
    </header>

    <div class="transcript-viewer-body">
      <aside class="transcript-viewer-list" aria-label={$t('transcriptViewer.listLabel')}>
        {#if loadingList}
          <div class="transcript-viewer-empty">{$t('transcriptViewer.loadingList')}</div>
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

      <section class="transcript-viewer-pane">
        {#if loadingTranscript}
          <div class="transcript-viewer-empty">{$t('transcriptViewer.loadingTranscript')}</div>
        {:else if !selectedResumeId}
          <div class="transcript-viewer-empty">{$t('transcriptViewer.pickOne')}</div>
        {:else if !transcriptText}
          <div class="transcript-viewer-empty">{$t('transcriptViewer.emptyTranscript')}</div>
        {:else}
          {#if truncated}
            <div class="transcript-viewer-truncated">{$t('transcriptViewer.truncated')}</div>
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
