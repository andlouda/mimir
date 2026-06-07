<script>
  import { onDestroy, onMount, tick } from 'svelte';
  import { t } from '../i18n.js';
  import { cleanTranscript } from '../transcript/cleanTranscript.js';
  import { getTranscriptContent, getTranscriptContentScrubbed, listTranscripts, deleteTranscript, getTranscriptDiskUsage } from '../transcript/transcriptApi.js';

  export let initialResumeId = '';
  export let initialLabel = '';
  export let onClose = () => {};
  export let onError = () => {};

  let modalEl;
  let viewerEl;
  // Element that owned focus before the modal opened; we return focus to it
  // on close so keyboard users don't get dropped at the top of the page.
  let triggerEl = null;

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

  // Delete confirmation
  let pendingDeleteId = '';
  let deleting = false;

  // Disk usage
  let diskUsage = { count: 0, totalBytes: 0 };

  // Search
  let searchOpen = false;
  let searchQuery = '';
  let searchMatchIndex = 0;
  let searchInputEl;

  $: searchMatches = (() => {
    if (!searchQuery || !displayText) return [];
    const q = searchQuery.toLowerCase();
    const text = displayText.toLowerCase();
    const indices = [];
    let pos = 0;
    while (pos < text.length) {
      const idx = text.indexOf(q, pos);
      if (idx === -1) break;
      indices.push(idx);
      pos = idx + 1;
    }
    return indices;
  })();

  $: if (searchQuery) searchMatchIndex = searchMatches.length > 0 ? 0 : -1;

  function scrollToMatch(index) {
    if (!viewerEl || !searchMatches.length) return;
    const text = displayText;
    const matchStart = searchMatches[index];
    if (matchStart == null) return;
    // Use Selection API to highlight the match
    const textNode = viewerEl.firstChild;
    if (!textNode) return;
    try {
      const range = document.createRange();
      range.setStart(textNode, matchStart);
      range.setEnd(textNode, matchStart + searchQuery.length);
      const sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
      // scrollIntoView for the range
      const rect = range.getBoundingClientRect();
      const containerRect = viewerEl.getBoundingClientRect();
      if (rect.top < containerRect.top || rect.bottom > containerRect.bottom) {
        const scrollTarget = viewerEl.scrollTop + rect.top - containerRect.top - containerRect.height / 3;
        viewerEl.scrollTo({ top: scrollTarget, behavior: 'smooth' });
      }
    } catch (_) { /* text node length mismatch edge case */ }
  }

  function searchNext() {
    if (!searchMatches.length) return;
    searchMatchIndex = (searchMatchIndex + 1) % searchMatches.length;
    scrollToMatch(searchMatchIndex);
  }

  function searchPrev() {
    if (!searchMatches.length) return;
    searchMatchIndex = (searchMatchIndex - 1 + searchMatches.length) % searchMatches.length;
    scrollToMatch(searchMatchIndex);
  }

  function toggleSearch() {
    searchOpen = !searchOpen;
    if (searchOpen) {
      tick().then(() => searchInputEl?.focus());
    } else {
      searchQuery = '';
      window.getSelection()?.removeAllRanges();
    }
  }

  function handleSearchKeydown(e) {
    if (e.key === 'Enter') {
      e.preventDefault();
      if (e.shiftKey) searchPrev();
      else searchNext();
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      toggleSearch();
    }
  }

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
    if (resumeId === selectedResumeId) return;
    selectedResumeId = resumeId;
  }

  function confirmDelete(resumeId) {
    pendingDeleteId = resumeId;
  }

  function cancelDelete() {
    pendingDeleteId = '';
  }

  async function executeDelete() {
    if (!pendingDeleteId) return;
    deleting = true;
    try {
      const result = await deleteTranscript(pendingDeleteId);
      if (result.deleted) {
        entries = entries.filter(e => e.resumeId !== pendingDeleteId);
        if (selectedResumeId === pendingDeleteId) {
          selectedResumeId = entries.length > 0 ? entries[0].resumeId : '';
          if (!selectedResumeId) {
            transcriptText = '';
            transcriptSize = 0;
            truncated = false;
          }
        }
        await refreshDiskUsage();
      } else if (result.error) {
        onError(result.error);
      }
    } catch (error) {
      onError(`Delete failed: ${error?.message || error}`);
    } finally {
      pendingDeleteId = '';
      deleting = false;
    }
  }

  async function refreshDiskUsage() {
    try {
      diskUsage = await getTranscriptDiskUsage();
    } catch (_) { /* non-critical */ }
  }

  // Keyboard navigation for the transcript list (P1-5)
  function handleListKeydown(e) {
    if (!entries.length) return;
    const currentIdx = entries.findIndex(en => en.resumeId === selectedResumeId);
    let nextIdx = currentIdx;
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        nextIdx = Math.min(currentIdx + 1, entries.length - 1);
        break;
      case 'ArrowUp':
        e.preventDefault();
        nextIdx = Math.max(currentIdx - 1, 0);
        break;
      case 'Home':
        e.preventDefault();
        nextIdx = 0;
        break;
      case 'End':
        e.preventDefault();
        nextIdx = entries.length - 1;
        break;
      case 'Enter':
        e.preventDefault();
        if (currentIdx >= 0) select(entries[currentIdx].resumeId);
        return;
      case 'Delete':
      case 'Backspace':
        e.preventDefault();
        if (currentIdx >= 0 && !entries[currentIdx].active) {
          confirmDelete(entries[currentIdx].resumeId);
        }
        return;
      default:
        return;
    }
    if (nextIdx !== currentIdx && nextIdx >= 0) {
      selectedResumeId = entries[nextIdx].resumeId;
      // Scroll the entry into view
      tick().then(() => {
        const listEl = modalEl?.querySelector('.transcript-viewer-list ul');
        const activeEl = listEl?.querySelector('.transcript-viewer-entry.active');
        activeEl?.scrollIntoView({ block: 'nearest' });
      });
    }
  }

  async function copyAll() {
    if (!transcriptText) return;
    try {
      await navigator.clipboard.writeText(displayText);
    } catch (error) {
      onError(`Copy failed: ${error?.message || error}`);
    }
  }

  async function copyScrubbed() {
    if (!selectedResumeId) return;
    try {
      const result = await getTranscriptContentScrubbed(selectedResumeId);
      const text = showCleaned ? cleanTranscript(result.text) : result.text;
      await navigator.clipboard.writeText(text);
    } catch (error) {
      onError(`Copy failed: ${error?.message || error}`);
    }
  }

  // Focus-trap: every Tab/Shift+Tab loops at the first/last focusable child
  // of the modal. Keeps keyboard users from escaping the dialog while it's
  // open. Cheap, no external library.
  const FOCUSABLE_SELECTOR = [
    'a[href]',
    'button:not([disabled])',
    'input:not([disabled])',
    'select:not([disabled])',
    'textarea:not([disabled])',
    '[tabindex]:not([tabindex="-1"])',
  ].join(',');

  function focusableElements() {
    if (!modalEl) return [];
    return Array.from(modalEl.querySelectorAll(FOCUSABLE_SELECTOR)).filter(
      (el) => !el.hasAttribute('disabled') && el.offsetParent !== null
    );
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      if (searchOpen) { toggleSearch(); return; }
      if (pendingDeleteId) { cancelDelete(); return; }
      onClose();
      return;
    }
    if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
      e.preventDefault();
      toggleSearch();
      return;
    }
    if (e.key !== 'Tab') return;
    const focusables = focusableElements();
    if (focusables.length === 0) {
      e.preventDefault();
      modalEl?.focus();
      return;
    }
    const first = focusables[0];
    const last = focusables[focusables.length - 1];
    const active = document.activeElement;
    if (e.shiftKey && active === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && active === last) {
      e.preventDefault();
      first.focus();
    }
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
    triggerEl = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    document.addEventListener('mousedown', handleDocumentPointer, true);
    document.addEventListener('touchstart', handleDocumentPointer, true);
    await tick();
    // Move focus *into* the modal so screen readers announce it and Tab
    // navigation starts inside. Prefer the dialog itself so the
    // Tab-trap-first-element logic doesn't immediately jump to the close X.
    modalEl?.focus();
    // The reactive `$: if (selectedResumeId) loadTranscript(...)` below is
    // the single source of truth for the load — calling it explicitly here
    // would race with reactivity-after-loadList and fire two IPC calls.
    await loadList();
    refreshDiskUsage();
  });

  onDestroy(() => {
    document.removeEventListener('mousedown', handleDocumentPointer, true);
    document.removeEventListener('touchstart', handleDocumentPointer, true);
    // Restore focus to whoever opened the modal. Defensive: the element may
    // have been removed from the DOM by other reactivity while the modal
    // was open; the check guards against that.
    if (triggerEl && typeof triggerEl.focus === 'function' && document.contains(triggerEl)) {
      triggerEl.focus();
    }
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
          <ul role="listbox" tabindex="0" on:keydown={handleListKeydown} aria-activedescendant={selectedResumeId ? `transcript-${selectedResumeId}` : undefined}>
            {#each entries as entry (entry.resumeId)}
              <li>
                <button
                  type="button"
                  id="transcript-{entry.resumeId}"
                  role="option"
                  aria-selected={entry.resumeId === selectedResumeId}
                  class="transcript-viewer-entry"
                  class:active={entry.resumeId === selectedResumeId}
                  on:click={() => select(entry.resumeId)}
                >
                  <span class="transcript-entry-label">
                    {entryLabel(entry)}
                    {#if entry.active}
                      <span class="transcript-entry-badge">{$t('transcriptViewer.activeBadge')}</span>
                    {/if}
                  </span>
                  <span class="transcript-entry-meta">
                    {formatRelative(entry.modTime)} · {formatBytes(entry.size)}
                    {#if !entry.active}
                      <span
                        role="button"
                        tabindex="-1"
                        class="transcript-delete-btn"
                        on:click|stopPropagation={() => confirmDelete(entry.resumeId)}
                        on:keydown|stopPropagation={(e) => { if (e.key === 'Enter' || e.key === ' ') confirmDelete(entry.resumeId); }}
                        title={$t('transcriptViewer.deleteTitle')}
                      >&#x2715;</span>
                    {/if}
                  </span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </aside>
      {/if}

      <section class="transcript-viewer-pane">
        {#if searchOpen && transcriptText}
          <div class="transcript-search-bar">
            <input
              bind:this={searchInputEl}
              type="text"
              bind:value={searchQuery}
              on:keydown={handleSearchKeydown}
              placeholder={$t('transcriptViewer.searchPlaceholder')}
              class="transcript-search-input"
            />
            <span class="transcript-search-count">
              {#if searchQuery}
                {searchMatches.length > 0 ? `${searchMatchIndex + 1}/${searchMatches.length}` : $t('transcriptViewer.noMatches')}
              {/if}
            </span>
            <button type="button" class="transcript-search-nav" on:click={searchPrev} disabled={!searchMatches.length} title={$t('transcriptViewer.searchPrev')}>&#x25B2;</button>
            <button type="button" class="transcript-search-nav" on:click={searchNext} disabled={!searchMatches.length} title={$t('transcriptViewer.searchNext')}>&#x25BC;</button>
            <button type="button" class="transcript-search-nav" on:click={toggleSearch} title={$t('transcriptViewer.close')}>&#x2715;</button>
          </div>
        {/if}
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

      {#if pendingDeleteId}
        <div class="transcript-delete-overlay">
          <div class="transcript-delete-confirm">
            <p>{$t('transcriptViewer.deleteConfirm')}</p>
            <div class="transcript-delete-actions">
              <button type="button" class="modal-secondary-button" on:click={cancelDelete} disabled={deleting}>
                {$t('transcriptViewer.cancel')}
              </button>
              <button type="button" class="transcript-delete-confirm-btn" on:click={executeDelete} disabled={deleting}>
                {deleting ? $t('transcriptViewer.deleting') : $t('transcriptViewer.deleteAction')}
              </button>
            </div>
          </div>
        </div>
      {/if}
    </div>

    <footer class="transcript-viewer-footer">
      <div class="transcript-viewer-footer-left">
        <label class="transcript-viewer-toggle">
          <input type="checkbox" bind:checked={showRaw} />
          <span>{$t('transcriptViewer.showRaw')}</span>
        </label>
        {#if diskUsage.count > 0}
          <span class="transcript-disk-usage">
            {$t('transcriptViewer.diskUsage', { count: diskUsage.count, size: formatBytes(diskUsage.totalBytes) })}
          </span>
        {/if}
      </div>
      <div class="transcript-viewer-actions">
        <button
          type="button"
          class="modal-secondary-button"
          on:click={toggleSearch}
          disabled={!transcriptText}
          title="Ctrl+F"
        >
          {$t('transcriptViewer.search')}
        </button>
        <button
          type="button"
          class="modal-secondary-button"
          on:click={copyAll}
          disabled={!transcriptText}
        >
          {$t('transcriptViewer.copyAll')}
        </button>
        <button
          type="button"
          class="modal-secondary-button"
          on:click={copyScrubbed}
          disabled={!transcriptText}
          title={$t('transcriptViewer.copyScrubbed')}
        >
          {$t('transcriptViewer.copyScrubbed')}
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
    position: relative;
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
  .transcript-viewer-footer-left {
    display: flex;
    align-items: center;
    gap: 16px;
  }
  .transcript-disk-usage {
    font-size: 11px;
    color: #6f7787;
  }
  .transcript-entry-badge {
    display: inline-block;
    font-size: 9px;
    padding: 1px 5px;
    border-radius: 4px;
    background: #1a3a2a;
    color: #68d391;
    margin-left: 6px;
    vertical-align: middle;
    font-weight: 500;
  }
  .transcript-delete-btn {
    background: transparent;
    border: none;
    color: #6f7787;
    cursor: pointer;
    font-size: 10px;
    padding: 0 2px;
    margin-left: 4px;
    line-height: 1;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .transcript-viewer-entry:hover .transcript-delete-btn {
    opacity: 1;
  }
  .transcript-delete-btn:hover {
    color: #f56565;
  }
  .transcript-delete-overlay {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 10;
    border-radius: 10px;
  }
  .transcript-delete-confirm {
    background: #1a1d27;
    border: 1px solid #2b3140;
    border-radius: 8px;
    padding: 20px 24px;
    text-align: center;
    max-width: 340px;
  }
  .transcript-delete-confirm p {
    margin: 0 0 16px;
    font-size: 13px;
    color: #d6dae3;
  }
  .transcript-delete-actions {
    display: flex;
    gap: 8px;
    justify-content: center;
  }
  .transcript-delete-confirm-btn {
    background: #c53030;
    color: #fff;
    border: none;
    border-radius: 6px;
    padding: 6px 16px;
    font-size: 12px;
    cursor: pointer;
    font: inherit;
  }
  .transcript-delete-confirm-btn:hover {
    background: #e53e3e;
  }
  .transcript-delete-confirm-btn:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .transcript-search-bar {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    background: #1a1d27;
    border-bottom: 1px solid #2b3140;
  }
  .transcript-search-input {
    flex: 1;
    background: #0c0e14;
    border: 1px solid #2b3140;
    border-radius: 4px;
    color: #d6dae3;
    padding: 4px 8px;
    font: inherit;
    font-size: 12px;
    outline: none;
  }
  .transcript-search-input:focus {
    border-color: #63b3ed;
  }
  .transcript-search-count {
    font-size: 11px;
    color: #6f7787;
    min-width: 50px;
    text-align: center;
  }
  .transcript-search-nav {
    background: transparent;
    border: 1px solid #2b3140;
    border-radius: 4px;
    color: #c8cdd8;
    cursor: pointer;
    padding: 2px 6px;
    font-size: 10px;
    font: inherit;
    line-height: 1.2;
  }
  .transcript-search-nav:hover {
    background: #1c2230;
  }
  .transcript-search-nav:disabled {
    opacity: 0.4;
    cursor: default;
  }
</style>
