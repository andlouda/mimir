<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { marked } from 'marked';
  import { t } from './i18n.js';
  import { sanitizeHtml } from './util.js';

  const dispatch = createEventDispatcher();

  // Wails bindings accessed via window to avoid import-time errors
  const ListNotes = (...args) => window['go']['main']['App']['ListNotes'](...args);
  const GetNote = (...args) => window['go']['main']['App']['GetNote'](...args);
  const SaveNote = (...args) => window['go']['main']['App']['SaveNote'](...args);
  const DeleteNote = (...args) => window['go']['main']['App']['DeleteNote'](...args);
  const RenameNote = (...args) => window['go']['main']['App']['RenameNote'](...args);
  const ImportNoteFromLocal = (...args) => window['go']['main']['App']['ImportNoteFromLocal'](...args);
  const ImportNoteFromRemote = (...args) => window['go']['main']['App']['ImportNoteFromRemote'](...args);

  export let sshTerminals = [];

  let notes = [];
  let activeNote = null;
  let editorContent = '';
  let editorTab = 'edit';
  let errorMessage = '';
  let loading = false;
  let showImportMenu = false;
  let showNewNoteInput = false;
  let newNoteFilename = '';
  let showLocalImport = false;
  let localImportPath = '';
  let showRemoteImport = false;
  let remoteImportTerminalId = 0;
  let remoteImportPath = '';
  let renameTarget = null;
  let renameValue = '';
  let dirty = false;

  onMount(() => {
    loadNotes();
  });

  async function loadNotes() {
    loading = true;
    try {
      notes = await ListNotes();
      errorMessage = '';
    } catch (err) {
      errorMessage = `Failed to load notes: ${err}`;
    } finally {
      loading = false;
    }
  }

  async function openNote(filename) {
    if (dirty && activeNote) {
      await saveCurrentNote();
    }
    try {
      const result = await GetNote(filename);
      activeNote = result;
      editorContent = result.content;
      editorTab = 'edit';
      dirty = false;
      errorMessage = '';
    } catch (err) {
      errorMessage = `Failed to open note: ${err}`;
    }
  }

  async function saveCurrentNote() {
    if (!activeNote) return;
    try {
      await SaveNote(activeNote.filename, editorContent);
      activeNote = { ...activeNote, content: editorContent };
      dirty = false;
      await loadNotes();
      errorMessage = '';
    } catch (err) {
      errorMessage = `Failed to save: ${err}`;
    }
  }

  async function createNote() {
    let name = newNoteFilename.trim();
    if (!name) {
      errorMessage = $t('markdownNotes.emptyFilename') || 'Please enter a filename';
      return;
    }
    if (!name.endsWith('.md')) name += '.md';
    try {
      await SaveNote(name, '');
      showNewNoteInput = false;
      newNoteFilename = '';
      await loadNotes();
      await openNote(name);
    } catch (err) {
      errorMessage = `Failed to create note: ${err}`;
    }
  }

  async function deleteNote(filename) {
    try {
      await DeleteNote(filename);
      if (activeNote && activeNote.filename === filename) {
        activeNote = null;
        editorContent = '';
        dirty = false;
      }
      await loadNotes();
    } catch (err) {
      errorMessage = `Failed to delete: ${err}`;
    }
  }

  function startRename(note) {
    renameTarget = note.filename;
    renameValue = note.filename;
  }

  async function confirmRename() {
    if (!renameTarget || !renameValue.trim()) return;
    let newName = renameValue.trim();
    if (!newName.endsWith('.md')) newName += '.md';
    if (newName === renameTarget) {
      renameTarget = null;
      return;
    }
    try {
      await RenameNote(renameTarget, newName);
      if (activeNote && activeNote.filename === renameTarget) {
        activeNote = { ...activeNote, filename: newName };
      }
      renameTarget = null;
      renameValue = '';
      await loadNotes();
    } catch (err) {
      errorMessage = `Failed to rename: ${err}`;
    }
  }

  async function importLocal() {
    const path = localImportPath.trim();
    if (!path) return;
    try {
      await ImportNoteFromLocal(path);
      showLocalImport = false;
      localImportPath = '';
      showImportMenu = false;
      await loadNotes();
    } catch (err) {
      errorMessage = `Import failed: ${err}`;
    }
  }

  async function importRemote() {
    const path = remoteImportPath.trim();
    if (!path || !remoteImportTerminalId) return;
    try {
      await ImportNoteFromRemote(remoteImportTerminalId, path);
      showRemoteImport = false;
      remoteImportPath = '';
      remoteImportTerminalId = 0;
      showImportMenu = false;
      await loadNotes();
    } catch (err) {
      errorMessage = `Remote import failed: ${err}`;
    }
  }

  function insertMarkdown(prefix, suffix = '') {
    const textarea = document.querySelector('.note-editor-textarea');
    if (!textarea) return;
    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const selected = editorContent.substring(start, end);
    const before = editorContent.substring(0, start);
    const after = editorContent.substring(end);
    editorContent = before + prefix + selected + suffix + after;
    dirty = true;
    // Restore cursor position after the inserted prefix
    requestAnimationFrame(() => {
      textarea.focus();
      const newPos = start + prefix.length + selected.length + suffix.length;
      textarea.setSelectionRange(newPos, newPos);
    });
  }

  function goBackToList() {
    if (dirty && activeNote) {
      saveCurrentNote();
    }
    activeNote = null;
    editorContent = '';
    dirty = false;
  }

  function formatTimeAgo(unix) {
    const diff = Math.floor(Date.now() / 1000) - unix;
    if (diff < 60) return 'just now';
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    return `${Math.floor(diff / 86400)}d ago`;
  }

  function handleEditorInput() {
    dirty = true;
  }

  function handleEditorKeydown(e) {
    if (e.ctrlKey && e.key === 's') {
      e.preventDefault();
      saveCurrentNote();
    }
  }

  function autofocusAction(node) {
    requestAnimationFrame(() => node.focus());
  }

  $: renderedMarkdown = activeNote && editorTab === 'preview' ? sanitizeHtml(marked(editorContent || '')) : '';
</script>

<div class="notes-container">
  <!-- Header -->
  <div class="notes-header">
    {#if activeNote}
      <button class="notes-back-btn" on:click={goBackToList} title={$t('markdownNotes.backToList')}>&larr;</button>
      <span class="notes-title" title={activeNote.filename}>{activeNote.filename}</span>
    {:else}
      <span class="notes-title">{$t('markdownNotes.title')}</span>
    {/if}
    <div class="notes-header-actions">
      {#if !activeNote}
        <button class="notes-icon-btn" on:click={() => { showNewNoteInput = !showNewNoteInput; showImportMenu = false; }} title={$t('markdownNotes.newNote')}>+</button>
        <div class="import-wrap">
          <button class="notes-icon-btn" on:click={() => { showImportMenu = !showImportMenu; showNewNoteInput = false; }} title={$t('markdownNotes.import')}>&#x21E9;</button>
          {#if showImportMenu}
            <div class="import-menu">
              <button class="import-menu-item" on:click={() => { showLocalImport = true; showRemoteImport = false; }}>{$t('markdownNotes.localFile')}</button>
              <button class="import-menu-item" on:click={() => { showRemoteImport = true; showLocalImport = false; }}>{$t('markdownNotes.fromSSH')}</button>
            </div>
          {/if}
        </div>
      {/if}
      <button class="notes-icon-btn notes-close-btn" on:click={() => dispatch('close')} title={$t('markdownNotes.close')}>&times;</button>
    </div>
  </div>

  {#if errorMessage}
    <div class="notes-error">
      {errorMessage}
      <button class="notes-error-dismiss" on:click={() => errorMessage = ''}>&times;</button>
    </div>
  {/if}

  <!-- New Note Input -->
  {#if showNewNoteInput && !activeNote}
    <div class="notes-input-row">
      <input
        type="text"
        bind:value={newNoteFilename}
        placeholder={$t('markdownNotes.filenamePlaceholder')}
        use:autofocusAction
        on:keydown={(e) => { if (e.key === 'Enter') createNote(); if (e.key === 'Escape') { showNewNoteInput = false; newNoteFilename = ''; }}}
      />
      <button on:click={createNote}>{$t('markdownNotes.create')}</button>
    </div>
  {/if}

  <!-- Local Import -->
  {#if showLocalImport && !activeNote}
    <div class="notes-input-row">
      <input type="text" bind:value={localImportPath} placeholder={$t('markdownNotes.localPathPlaceholder')} on:keydown={(e) => { if (e.key === 'Enter') importLocal(); if (e.key === 'Escape') { showLocalImport = false; localImportPath = ''; }}} />
      <button on:click={importLocal}>{$t('markdownNotes.import')}</button>
    </div>
  {/if}

  <!-- Remote Import -->
  {#if showRemoteImport && !activeNote}
    <div class="notes-input-row remote-import">
      <select bind:value={remoteImportTerminalId}>
        <option value={0}>{$t('markdownNotes.selectSSH')}</option>
        {#each sshTerminals as term}
          <option value={term.id}>{term.name || `SSH #${term.id}`}</option>
        {/each}
      </select>
      <input type="text" bind:value={remoteImportPath} placeholder={$t('markdownNotes.remotePathPlaceholder')} on:keydown={(e) => { if (e.key === 'Enter') importRemote(); if (e.key === 'Escape') { showRemoteImport = false; remoteImportPath = ''; }}} />
      <button on:click={importRemote}>{$t('markdownNotes.import')}</button>
    </div>
  {/if}

  <!-- Note List -->
  {#if !activeNote}
    <div class="notes-list">
      {#if loading}
        <div class="notes-empty">{$t('markdownNotes.loading')}</div>
      {:else if notes.length === 0}
        <div class="notes-empty">{$t('markdownNotes.empty')}</div>
      {:else}
        {#each notes as note (note.filename)}
          <div class="note-item" on:click={() => openNote(note.filename)} on:keydown={(e) => { if (e.key === 'Enter') openNote(note.filename); }} tabindex="0" role="button">
            {#if renameTarget === note.filename}
              <input
                class="rename-input"
                bind:value={renameValue}
                on:keydown|stopPropagation={(e) => { if (e.key === 'Enter') confirmRename(); if (e.key === 'Escape') { renameTarget = null; }}}
                on:click|stopPropagation
                on:blur={confirmRename}
              />
            {:else}
              <div class="note-item-info">
                <span class="note-item-title">{note.title}</span>
                <span class="note-item-meta">{formatTimeAgo(note.modTime)}</span>
              </div>
              <div class="note-item-actions">
                <button class="note-action-btn" on:click|stopPropagation={() => startRename(note)} title={$t('markdownNotes.rename')}>&#x270E;</button>
                <button class="note-action-btn note-delete-btn" on:click|stopPropagation={() => deleteNote(note.filename)} title={$t('markdownNotes.delete')}>&times;</button>
              </div>
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  {/if}

  <!-- Editor -->
  {#if activeNote}
    <div class="note-editor">
      <div class="note-editor-tabs">
        <button class:active={editorTab === 'edit'} on:click={() => { editorTab = 'edit'; }}>{$t('markdownNotes.editTab')}</button>
        <button class:active={editorTab === 'preview'} on:click={() => { editorTab = 'preview'; }}>{$t('markdownNotes.previewTab')}</button>
        <div class="note-editor-right">
          {#if dirty}
            <span class="dirty-indicator">*</span>
          {/if}
          <button class="save-btn" on:click={saveCurrentNote} title={$t('markdownNotes.saveTitle')}>{$t('markdownNotes.save')}</button>
        </div>
      </div>

      {#if editorTab === 'edit'}
        <div class="note-toolbar">
          <button on:click={() => insertMarkdown('**', '**')} title={$t('markdownNotes.bold')}><b>B</b></button>
          <button on:click={() => insertMarkdown('*', '*')} title={$t('markdownNotes.italic')}><i>I</i></button>
          <button on:click={() => insertMarkdown('## ')} title={$t('markdownNotes.heading')}>H</button>
          <button on:click={() => insertMarkdown('`', '`')} title={$t('markdownNotes.code')}>&lt;/&gt;</button>
          <button on:click={() => insertMarkdown('[', '](url)')} title={$t('markdownNotes.link')}>&#x1F517;</button>
          <button on:click={() => insertMarkdown('- ')} title={$t('markdownNotes.list')}>&#x2022;</button>
        </div>
        <textarea
          class="note-editor-textarea"
          bind:value={editorContent}
          on:input={handleEditorInput}
          on:keydown={handleEditorKeydown}
          placeholder={$t('markdownNotes.editorPlaceholder')}
          spellcheck="false"
        ></textarea>
      {:else}
        <div class="note-preview markdown-body">
          {@html renderedMarkdown}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .notes-container {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: var(--bg-deep);
    color: var(--text-primary);
    overflow: hidden;
  }

  .notes-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.65rem;
    border-bottom: 1px solid var(--border-subtle);
    flex-shrink: 0;
    min-height: 36px;
  }

  .notes-title {
    font-size: 0.82rem;
    font-weight: 600;
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .notes-header-actions {
    display: flex;
    gap: 0.25rem;
    align-items: center;
    flex-shrink: 0;
  }

  .notes-back-btn {
    background: none;
    border: 1px solid var(--border-dim);
    color: var(--text-secondary);
    cursor: pointer;
    padding: 0.15rem 0.4rem;
    border-radius: var(--radius-sm);
    font-size: 0.8rem;
  }
  .notes-back-btn:hover {
    color: var(--text-primary);
    background: var(--accent-glow);
  }

  .notes-icon-btn {
    background: none;
    border: 1px solid var(--border-dim);
    color: var(--text-secondary);
    cursor: pointer;
    width: 26px;
    height: 26px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-sm);
    font-size: 0.9rem;
    line-height: 1;
  }
  .notes-icon-btn:hover {
    color: var(--text-primary);
    background: var(--accent-glow);
  }
  .notes-close-btn:hover {
    color: #e55;
  }

  .import-wrap {
    position: relative;
  }
  .import-menu {
    position: absolute;
    top: 100%;
    right: 0;
    margin-top: 4px;
    background: var(--bg-surface);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    z-index: 100;
    min-width: 120px;
    box-shadow: 0 4px 12px rgba(0,0,0,0.4);
  }
  .import-menu-item {
    display: block;
    width: 100%;
    padding: 0.45rem 0.7rem;
    background: none;
    border: none;
    border-bottom: 1px solid var(--border-dim);
    color: var(--text-secondary);
    font-size: 0.78rem;
    cursor: pointer;
    text-align: left;
  }
  .import-menu-item:last-child { border-bottom: none; }
  .import-menu-item:hover {
    background: var(--accent-glow);
    color: var(--text-primary);
  }

  .notes-error {
    padding: 0.4rem 0.6rem;
    background: rgba(220, 50, 50, 0.15);
    color: #f88;
    font-size: 0.75rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-shrink: 0;
  }
  .notes-error-dismiss {
    background: none;
    border: none;
    color: #f88;
    cursor: pointer;
    font-size: 1rem;
  }

  .notes-input-row {
    display: flex;
    gap: 0.35rem;
    padding: 0.45rem 0.65rem;
    border-bottom: 1px solid var(--border-dim);
    flex-shrink: 0;
  }
  .notes-input-row input,
  .notes-input-row select {
    flex: 1;
    background: var(--bg-void);
    border: 1px solid var(--border-dim);
    color: var(--text-primary);
    padding: 0.3rem 0.5rem;
    font-size: 0.78rem;
    border-radius: var(--radius-sm);
    font-family: var(--font-mono);
  }
  .notes-input-row button {
    background: var(--accent-glow);
    border: 1px solid var(--border-dim);
    color: var(--text-primary);
    padding: 0.3rem 0.6rem;
    font-size: 0.75rem;
    cursor: pointer;
    border-radius: var(--radius-sm);
  }
  .notes-input-row button:hover {
    background: var(--accent);
    color: var(--bg-deep);
  }
  .remote-import {
    flex-wrap: wrap;
  }
  .remote-import select {
    flex: 1 1 100%;
    margin-bottom: 0.25rem;
  }

  .notes-list {
    flex: 1;
    overflow-y: auto;
    min-height: 0;
  }

  .notes-empty {
    padding: 1.5rem 0.8rem;
    text-align: center;
    color: var(--text-muted);
    font-size: 0.78rem;
  }

  .note-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 0.65rem;
    border-bottom: 1px solid var(--border-dim);
    cursor: pointer;
    transition: background 0.12s;
  }
  .note-item:hover {
    background: var(--accent-glow);
  }

  .note-item-info {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    overflow: hidden;
    flex: 1;
    min-width: 0;
  }
  .note-item-title {
    font-size: 0.8rem;
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .note-item-meta {
    font-size: 0.68rem;
    color: var(--text-muted);
  }

  .note-item-actions {
    display: flex;
    gap: 0.15rem;
    opacity: 0;
    transition: opacity 0.12s;
    flex-shrink: 0;
  }
  .note-item:hover .note-item-actions {
    opacity: 1;
  }

  .note-action-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    font-size: 0.85rem;
    padding: 0.1rem 0.25rem;
    border-radius: var(--radius-sm);
  }
  .note-action-btn:hover {
    color: var(--text-primary);
    background: var(--accent-glow);
  }
  .note-delete-btn:hover {
    color: #e55;
  }

  .rename-input {
    flex: 1;
    background: var(--bg-void);
    border: 1px solid var(--accent);
    color: var(--text-primary);
    padding: 0.25rem 0.4rem;
    font-size: 0.78rem;
    border-radius: var(--radius-sm);
    font-family: var(--font-mono);
  }

  /* Editor */
  .note-editor {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .note-editor-tabs {
    display: flex;
    align-items: center;
    gap: 0;
    border-bottom: 1px solid var(--border-dim);
    flex-shrink: 0;
    padding: 0 0.65rem;
  }
  .note-editor-tabs button {
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--text-muted);
    padding: 0.4rem 0.7rem;
    font-size: 0.76rem;
    cursor: pointer;
    transition: color 0.12s, border-color 0.12s;
  }
  .note-editor-tabs button:hover {
    color: var(--text-primary);
  }
  .note-editor-tabs button.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
  }

  .note-editor-right {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 0.3rem;
  }
  .dirty-indicator {
    color: var(--accent);
    font-size: 1rem;
    font-weight: bold;
  }
  .save-btn {
    background: var(--accent-glow) !important;
    border: 1px solid var(--border-dim) !important;
    color: var(--text-primary) !important;
    padding: 0.2rem 0.55rem !important;
    font-size: 0.72rem !important;
    cursor: pointer;
    border-radius: var(--radius-sm) !important;
  }
  .save-btn:hover {
    background: var(--accent) !important;
    color: var(--bg-deep) !important;
  }

  .note-toolbar {
    display: flex;
    gap: 0.2rem;
    padding: 0.3rem 0.65rem;
    border-bottom: 1px solid var(--border-dim);
    flex-shrink: 0;
  }
  .note-toolbar button {
    background: none;
    border: 1px solid var(--border-dim);
    color: var(--text-muted);
    cursor: pointer;
    width: 26px;
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-sm);
    font-size: 0.75rem;
  }
  .note-toolbar button:hover {
    color: var(--text-primary);
    background: var(--accent-glow);
  }

  .note-editor-textarea {
    flex: 1;
    width: 100%;
    resize: none;
    background: var(--bg-void);
    color: var(--text-primary);
    border: none;
    padding: 0.6rem 0.65rem;
    font-size: 0.82rem;
    font-family: var(--font-mono);
    line-height: 1.55;
    outline: none;
    min-height: 0;
  }
  .note-editor-textarea::placeholder {
    color: var(--text-muted);
  }

  .note-preview {
    flex: 1;
    overflow-y: auto;
    padding: 0.6rem 0.65rem;
    font-size: 0.82rem;
    line-height: 1.6;
    min-height: 0;
  }
  .note-preview :global(h1) { font-size: 1.3rem; margin: 0.5rem 0; color: var(--accent); }
  .note-preview :global(h2) { font-size: 1.1rem; margin: 0.4rem 0; color: var(--accent); }
  .note-preview :global(h3) { font-size: 0.95rem; margin: 0.35rem 0; }
  .note-preview :global(p) { margin: 0.4rem 0; }
  .note-preview :global(code) {
    background: var(--bg-surface);
    padding: 0.1rem 0.35rem;
    border-radius: 3px;
    font-family: var(--font-mono);
    font-size: 0.78rem;
  }
  .note-preview :global(pre) {
    background: var(--bg-void);
    padding: 0.6rem;
    border-radius: var(--radius-sm);
    overflow-x: auto;
    border: 1px solid var(--border-dim);
  }
  .note-preview :global(pre code) {
    background: none;
    padding: 0;
  }
  .note-preview :global(a) {
    color: var(--accent);
    text-decoration: underline;
  }
  .note-preview :global(ul), .note-preview :global(ol) {
    padding-left: 1.2rem;
    margin: 0.3rem 0;
  }
  .note-preview :global(li) { margin: 0.15rem 0; }
  .note-preview :global(blockquote) {
    border-left: 3px solid var(--accent);
    padding: 0.2rem 0.6rem;
    margin: 0.4rem 0;
    color: var(--text-secondary);
    background: var(--accent-glow);
  }
  .note-preview :global(hr) {
    border: none;
    border-top: 1px solid var(--border-dim);
    margin: 0.8rem 0;
  }
  .note-preview :global(table) {
    border-collapse: collapse;
    width: 100%;
    margin: 0.4rem 0;
  }
  .note-preview :global(th), .note-preview :global(td) {
    border: 1px solid var(--border-dim);
    padding: 0.3rem 0.5rem;
    font-size: 0.78rem;
    text-align: left;
  }
  .note-preview :global(th) {
    background: var(--bg-surface);
  }
</style>
