<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { ListDirectory, GetFileContent, OpenPathInExplorer, GetCurrentDirectory, RemoteListDirectory, RemoteGetFileContent, RemoteGetHomeDir } from '../../wailsjs/go/main/App';
  import { t } from './i18n.js';

  const dispatch = createEventDispatcher();

  export let currentPath = '';
  export let remoteTerminalId = 0;
  export let remoteLabel = '';

  let files = [];
  let errorMessage = '';
  let fileContent = '';
  let activeFilePath = '';
  let showFileContentModal = false;

  $: isRemote = remoteTerminalId > 0;

  // Helper function to normalize paths (e.g., replace backslashes with forward slashes)
  function normalizePath(path) {
    return path.replace(/\\/g, '/');
  }

  // Helper function to join path segments safely
  function joinPaths(base, segment) {
    base = normalizePath(base);
    segment = normalizePath(segment);

    // Handle absolute paths for segment (e.g., /home or C:/)
    if (segment.startsWith('/') || segment.match(/^[A-Z]:\//i)) {
      return segment; // Segment is already an absolute path
    }

    // Remove trailing slash from base if it's not a root directory
    if (base.endsWith('/') && base.length > 1 && !base.match(/^[A-Z]:\/$/i)) {
      base = base.slice(0, -1);
    }

    // Handle Windows drive roots (e.g., C:)
    if (base.match(/^[A-Z]:$/i)) {
      return base + '/' + segment;
    }

    return `${base}/${segment}`.replace(/\/\/+/g, '/'); // Replace multiple slashes with single
  }

  // Helper function to get the parent path
  function getParentPath(path) {
    path = normalizePath(path);

    // If at root (Linux) or drive root (Windows, e.g., C:/), stay there
    if (path === '/' || path.match(/^[A-Z]:\/$/i)) {
      return path;
    }

    const lastSlashIndex = path.lastIndexOf('/');
    if (lastSlashIndex > -1) {
      let parent = path.substring(0, lastSlashIndex);
      // If parent becomes empty, it means we are at a top-level directory like /folder or C:/folder
      if (parent === '') {
        // Check if it's a Windows drive root (e.g., C:/)
        if (path.match(/^[A-Z]:\/.*$/i)) {
          return path.substring(0, 3); // Return C:/
        } else {
          return '/'; // Return Linux root
        }
      }
      return parent;
    }
    // Should not happen if path is normalized and not a root
    return '/';
  }

  async function loadDirectory(path) {
    try {
      errorMessage = '';
      currentPath = normalizePath(path);
      if (isRemote) {
        files = await RemoteListDirectory(remoteTerminalId, currentPath);
      } else {
        files = await ListDirectory(currentPath);
      }
      files.sort((a, b) => {
        if (a.isDir === b.isDir) {
          return a.name.localeCompare(b.name);
        }
        return a.isDir ? -1 : 1;
      });
    } catch (error) {
      errorMessage = `Failed to load directory: ${error.message || error}`;
      files = [];
    }
  }

  function navigateTo(name) {
    loadDirectory(joinPaths(currentPath, name));
  }

  function navigateUp() {
    loadDirectory(getParentPath(currentPath));
  }

  async function viewFileContent(filePath) {
    try {
      activeFilePath = filePath;
      if (isRemote) {
        fileContent = await RemoteGetFileContent(remoteTerminalId, filePath);
      } else {
        fileContent = await GetFileContent(filePath);
      }
      showFileContentModal = true;
    } catch (error) {
      errorMessage = `Failed to get file content: ${error.message || error}`;
      activeFilePath = '';
      fileContent = '';
    }
  }

  function isMarkdownFile(name) {
    return name.toLowerCase().endsWith('.md');
  }

  async function sendFileToTerminal(filePath, mode = 'plain') {
    try {
      let content;
      if (isRemote) {
        content = await RemoteGetFileContent(remoteTerminalId, filePath);
      } else {
        content = await GetFileContent(filePath);
      }
      const payload = mode === 'markdown'
        ? `File: ${filePath}\n\n\`\`\`\n${content}\n\`\`\``
        : content;
      dispatch('insertIntoTerminal', { path: filePath, content: payload });
    } catch (error) {
      errorMessage = `Failed to prepare file for terminal: ${error.message || error}`;
    }
  }

  function openInNotes(filePath) {
    dispatch('openInNotes', {
      path: filePath,
      remote: isRemote,
      terminalId: isRemote ? remoteTerminalId : 0,
    });
  }

  async function openInExplorer(filePath) {
    if (isRemote) return; // No explorer for remote files
    try {
      await OpenPathInExplorer(filePath);
    } catch (error) {
      errorMessage = `Failed to open in explorer: ${error.message || error}`;
    }
  }

  function handleOpenTerminalHere(dirPath) {
    if (isRemote) {
      // Send cd command to SSH terminal instead of opening new terminal
      dispatch('remoteCD', { terminalId: remoteTerminalId, path: dirPath });
    } else {
      dispatch('openTerminalHere', dirPath);
    }
  }

  // React to remote terminal changes
  let lastRemoteTerminalId = 0;
  $: if (remoteTerminalId !== lastRemoteTerminalId) {
    lastRemoteTerminalId = remoteTerminalId;
    initLoad();
  }

  async function initLoad() {
    try {
      if (isRemote) {
        const homePath = await RemoteGetHomeDir(remoteTerminalId);
        await loadDirectory(homePath);
      } else {
        const initialPath = await GetCurrentDirectory();
        await loadDirectory(initialPath);
      }
    } catch (error) {
      errorMessage = `Failed to load initial directory: ${error.message || error}`;
    }
  }

  // Initial load
  onMount(initLoad);

  // Expose loadDirectory for parent component to call
  export function setPath(path) {
    loadDirectory(path);
  }
</script>

<div class="file-browser">
  {#if isRemote}
    <div class="remote-banner">
      <span class="remote-dot"></span>
      {$t('fileBrowser.remote', { label: remoteLabel })}
      <button class="switch-btn" on:click={() => dispatch('switchToLocal')}>{$t('fileBrowser.switchToLocal')}</button>
    </div>
  {/if}

  {#if errorMessage}
    <div class="error-message">{errorMessage}</div>
  {/if}

  <div class="path-bar">
    <button
      on:click={navigateUp}
      on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') navigateUp(); }}
      disabled={currentPath === '/' || currentPath.match(/^[A-Z]:\/$/i)}
      class="nav-button"
    >{$t('fileBrowser.up')}</button>
    <span>{currentPath}</span>
  </div>

  <ul class="file-list">
    {#each files as file (file.name)}
      <li class="file-item" class:is-dir={file.isDir}>
        <span
          class="file-name"
          on:click={() => file.isDir ? navigateTo(file.name) : viewFileContent(joinPaths(currentPath, file.name))}
          on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { file.isDir ? navigateTo(file.name) : viewFileContent(joinPaths(currentPath, file.name)); }}}
          tabindex="0"
          role="button"
        >
          {file.isDir ? '📁' : '📄'} {file.name}
        </span>
        <div class="file-actions">
          {#if file.isDir}
            <button on:click={() => handleOpenTerminalHere(joinPaths(currentPath, file.name))} title={isRemote ? $t('fileBrowser.cdTitle') : $t('fileBrowser.openTerminalHereTitle')}>{isRemote ? $t('fileBrowser.cd') : $t('fileBrowser.openTerminalHere')}</button>
          {:else}
            <button on:click={() => viewFileContent(joinPaths(currentPath, file.name))} title={$t('fileBrowser.viewTitle')}>{$t('fileBrowser.view')}</button>
            {#if isMarkdownFile(file.name)}
              <button on:click={() => openInNotes(joinPaths(currentPath, file.name))} title={$t('fileBrowser.notesTitle')}>{$t('fileBrowser.notes')}</button>
            {:else}
              <button on:click={() => sendFileToTerminal(joinPaths(currentPath, file.name))} title={$t('fileBrowser.insertTitle')}>{$t('fileBrowser.insert')}</button>
            {/if}
          {/if}
          {#if !isRemote}
            <button on:click={() => openInExplorer(joinPaths(currentPath, file.name))} title={$t('fileBrowser.explorerTitle')}>{$t('fileBrowser.explorer')}</button>
          {/if}
        </div>
      </li>
    {/each}
  </ul>

  {#if showFileContentModal}
    <div class="modal-overlay" on:click={() => showFileContentModal = false} on:keydown={(e) => { if (e.key === 'Escape') showFileContentModal = false; }} tabindex="0" role="button">
      <div class="modal-content" role="dialog" aria-modal="true" tabindex="-1" on:click|stopPropagation on:keydown={(e) => e.stopPropagation()}>
        <h3>{$t('fileBrowser.fileContent')}</h3>
        <div class="modal-path">{activeFilePath}</div>
        <pre>{fileContent}</pre>
        <div class="modal-actions">
          {#if isMarkdownFile(activeFilePath)}
            <button on:click={() => { openInNotes(activeFilePath); showFileContentModal = false; }} class="secondary-button">{$t('fileBrowser.openInNotes')}</button>
          {:else}
            <button on:click={() => sendFileToTerminal(activeFilePath)} class="secondary-button">{$t('fileBrowser.insertPlain')}</button>
            <button on:click={() => sendFileToTerminal(activeFilePath, 'markdown')} class="secondary-button">{$t('fileBrowser.insertMarkdown')}</button>
          {/if}
          <button on:click={() => showFileContentModal = false}>{$t('fileBrowser.close')}</button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .file-browser {
    background-color: var(--bg-void);
    border: none;
    border-radius: 0;
    padding: 0;
    color: var(--text-primary);
    display: flex;
    flex-direction: column;
    height: 100%;
    font-family: var(--font-sans);
  }

  .remote-banner {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.45rem 0.8rem;
    background: rgba(126, 231, 135, 0.08);
    border-bottom: 1px solid rgba(126, 231, 135, 0.2);
    font-size: 0.78rem;
    font-weight: 600;
    color: #7ee787;
    flex-shrink: 0;
  }

  .remote-dot {
    display: inline-block;
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #7ee787;
    flex-shrink: 0;
  }

  .switch-btn {
    margin-left: auto;
    background: transparent;
    border: 1px solid rgba(126, 231, 135, 0.3);
    color: #7ee787;
    font-size: 0.7rem;
    font-weight: 500;
    padding: 0.2rem 0.5rem;
    border-radius: var(--radius-sm);
    cursor: pointer;
    transition: all var(--transition);
  }

  .switch-btn:hover {
    background: rgba(126, 231, 135, 0.15);
    border-color: rgba(126, 231, 135, 0.5);
  }

  .path-bar {
    display: flex;
    align-items: center;
    padding: 0.55rem 0.8rem;
    border-bottom: 1px solid var(--border-subtle);
    background: var(--bg-deep);
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .path-bar span {
    flex-grow: 1;
    font-family: var(--font-mono);
    font-size: 0.78rem;
    font-weight: 500;
    color: var(--text-secondary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .nav-button {
    background: var(--bg-surface);
    color: var(--text-secondary);
    border: 1px solid var(--border-dim);
    padding: 0.3rem 0.6rem;
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-family: var(--font-sans);
    font-size: 0.75rem;
    font-weight: 500;
    transition: all var(--transition);
    flex-shrink: 0;
  }

  .nav-button:hover {
    background: var(--accent-glow);
    color: var(--accent);
    border-color: var(--accent);
  }

  .nav-button:disabled {
    background: var(--bg-surface);
    color: var(--text-muted);
    border-color: var(--border-subtle);
    cursor: not-allowed;
    opacity: 0.5;
  }

  .file-list {
    list-style: none;
    padding: 0;
    margin: 0;
    overflow-y: auto;
    flex-grow: 1;
  }

  .file-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.4rem 0.8rem;
    border-bottom: 1px solid var(--border-subtle);
    transition: background var(--transition);
  }

  .file-item:hover {
    background: var(--accent-glow);
  }

  .file-item:last-child {
    border-bottom: none;
  }

  .file-name {
    cursor: pointer;
    flex-grow: 1;
    font-size: 0.82rem;
    color: var(--text-secondary);
    transition: color var(--transition);
    padding: 0.15rem 0;
  }

  .file-name:hover {
    color: var(--accent);
  }

  .is-dir .file-name {
    font-weight: 600;
    color: var(--text-primary);
  }

  .is-dir .file-name:hover {
    color: var(--accent);
  }

  .file-actions {
    display: flex;
    gap: 0.3rem;
    flex-shrink: 0;
    opacity: 0;
    transition: opacity var(--transition);
  }

  .file-item:hover .file-actions {
    opacity: 1;
  }

  .file-actions button {
    background: var(--bg-overlay);
    color: var(--text-secondary);
    border: 1px solid var(--border-dim);
    padding: 0.2rem 0.5rem;
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-family: var(--font-sans);
    font-size: 0.7rem;
    font-weight: 500;
    transition: all var(--transition);
  }

  .file-actions button:hover {
    background: var(--accent-glow);
    color: var(--accent);
    border-color: var(--accent);
  }

  .modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background-color: rgba(0, 0, 0, 0.65);
    backdrop-filter: blur(4px);
    display: flex;
    justify-content: center;
    align-items: center;
    z-index: 1000;
  }

  .modal-content {
    background-color: var(--bg-deep);
    padding: 1.5rem;
    border-radius: var(--radius-lg);
    max-width: 80%;
    max-height: 80%;
    overflow: auto;
    color: var(--text-primary);
    border: 1px solid var(--border-dim);
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5), 0 0 0 1px var(--border-subtle);
  }

  .modal-content h3 {
    color: var(--text-primary);
    font-size: 0.9rem;
    font-weight: 600;
    margin-bottom: 1rem;
    padding-bottom: 0.5rem;
    border-bottom: 1px solid var(--border-subtle);
  }

  .modal-content pre {
    white-space: pre-wrap;
    word-wrap: break-word;
    font-family: var(--font-mono);
    font-size: 0.78rem;
    line-height: 1.5;
    color: var(--text-secondary);
    background: var(--bg-surface);
    padding: 1rem;
    border-radius: var(--radius-md);
    border: 1px solid var(--border-subtle);
  }

  .modal-path {
    margin-bottom: 0.8rem;
    color: var(--text-muted);
    font-family: var(--font-mono);
    font-size: 0.72rem;
    word-break: break-all;
  }

  .modal-actions {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin-top: 1rem;
  }

  .modal-content button {
    background: var(--accent);
    color: var(--bg-void);
    border: none;
    padding: 0.4rem 1rem;
    border-radius: var(--radius-sm);
    cursor: pointer;
    font-family: var(--font-sans);
    font-size: 0.78rem;
    font-weight: 600;
    transition: all var(--transition);
  }

  .modal-content button:hover {
    background: var(--accent-hover);
    box-shadow: 0 0 12px var(--accent-glow);
  }

  .modal-content .secondary-button {
    background: var(--bg-overlay);
    color: var(--text-primary);
    border: 1px solid var(--border-dim);
  }

  .modal-content .secondary-button:hover {
    background: var(--accent-glow);
    color: var(--accent-hover);
    border-color: var(--accent);
    box-shadow: none;
  }

  .error-message {
    background: var(--error-bg);
    color: var(--error);
    padding: 0.55rem 0.8rem;
    margin: 0;
    border-bottom: 1px solid rgba(244, 112, 103, 0.15);
    font-size: 0.78rem;
    flex-shrink: 0;
  }
</style>
