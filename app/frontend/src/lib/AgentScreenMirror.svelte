<script>
  // Read-only xterm that shows what tmux captured from the agent's pane,
  // colours and box drawing included. Sized to the pane's width so lines
  // wrap exactly as in the pane; the history scrolls like a terminal.
  import { onDestroy, onMount } from 'svelte';
  import { Terminal } from '@xterm/xterm';
  import { XTERM_THEME } from './actions/terminalActions.js';
  import { terminalFontSize } from './stores/uiStore.js';

  export let text = '';
  export let width = 80;
  export let height = 24;

  let host;
  let term = null;
  let lastKey = '';

  function ensure() {
    if (term || !host) return;
    term = new Terminal({
      cols: Math.max(20, width || 80),
      rows: Math.max(5, Math.min(height || 24, 60)),
      disableStdin: true,
      cursorBlink: false,
      cursorStyle: 'underline',
      cursorInactiveStyle: 'none',
      scrollback: 5000,
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
      fontSize: Math.max(9, $terminalFontSize - 2),
      lineHeight: 1.2,
      theme: XTERM_THEME,
      allowProposedApi: true,
    });
    term.open(host);
  }

  function render() {
    ensure();
    if (!term) return;
    const key = `${width}x${height}:${text.length}:${text.slice(-200)}`;
    if (key === lastKey) return;
    lastKey = key;
    if (term.cols !== Math.max(20, width || 80) || term.rows !== Math.max(5, Math.min(height || 24, 60))) {
      term.resize(Math.max(20, width || 80), Math.max(5, Math.min(height || 24, 60)));
    }
    term.reset();
    term.write(String(text || '').replace(/\r?\n/g, '\r\n'), () => { try { term.scrollToBottom(); } catch { /* ignore */ } });
  }

  $: if (host) { text; width; height; render(); }

  onMount(render);
  onDestroy(() => { try { term?.dispose(); } catch { /* ignore */ } term = null; });
</script>

<div class="agent-screen-host" bind:this={host}></div>

<style>
  .agent-screen-host {
    overflow: auto;
    border: 1px solid var(--border-subtle);
    border-radius: 6px;
    background: var(--bg-void);
    padding: 4px;
    max-height: 70vh;
  }
  .agent-screen-host :global(.xterm) { width: max-content; }
</style>
