<script>
  import { t as tr } from '../i18n.js';
  import SplitPane from '../SplitPane.svelte';
  import MarkdownNotes from '../MarkdownNotes.svelte';

  export let availableTerminalTypes = [];
  export let selectedTerminalType = '';
  export let aiSettings = {};
  export let visibleTerminalCount = 0;
  export let errorMessage = '';
  export let layoutTree = null;
  export let terminalMap = new Map();
  export let activeTerminalId = null;
  export let notesPanelOpen = false;
  export let notesPanelWidth = 380;
  export let terminals = [];
  export let showAIMenu = false;

  export let persistDefaultTerminalType = () => {};
  export let addTerminal = () => {};
  export let toggleAIMenu = () => {};
  export let openAIPanel = () => {};
  export let dismissError = () => {};
  export let setActiveTerminalId = () => {};
  export let splitTerminal = () => {};
  export let toggleMinimize = () => {};
  export let removeTerminal = () => {};
  export let reconnectSSH = () => {};
  export let startEditingName = () => {};
  export let saveTerminalName = () => {};
  export let handleResize = () => {};
  export let touchLayoutTree = () => {};
  export let handleDragStart = () => {};
  export let handleDragOver = () => {};
  export let handleDragLeave = () => {};
  export let handleDrop = () => {};
  export let handleDragEnd = () => {};
  export let updateTerminalSearchQuery = () => {};
  export let terminalSearchNext = () => {};
  export let terminalSearchPrev = () => {};
  export let closeTerminalSearch = () => {};
  export let dismissRestoreSummary = () => {};
  export let toggleRecording = () => {};
  export let openTranscriptViewer = () => {};
  export let startNotesDrag = () => {};
  export let closeNotesPanel = () => {};
</script>

<div class="terminal-controls">
  <div class="controls-left">
    <select bind:value={selectedTerminalType} on:change={persistDefaultTerminalType} title={$tr('appTerminals.defaultTerminalType')}>
      {#each availableTerminalTypes as terminalTypeOption (terminalTypeOption.value)}
        <option value={terminalTypeOption.value}>{terminalTypeOption.label}</option>
      {/each}
    </select>
    <button on:click={addTerminal} class="add-btn">{$tr('appTerminals.newTerminal')}</button>
    <div class="ai-menu-wrap">
      <button type="button" on:click={toggleAIMenu} class="ai-btn">AI</button>
      {#if showAIMenu}
        <div class="ai-menu">
          <button type="button" on:click={() => openAIPanel('explain_output')} class="ai-menu-item" title={$tr('appTerminals.aiExplainTitle')}>
            <span>{$tr('appTerminals.aiExplain')}</span>
            <small>{$tr('appTerminals.aiExplainSub')}</small>
          </button>
          <button type="button" on:click={() => openAIPanel('suggest_next_command')} class="ai-menu-item" title={$tr('appTerminals.aiSuggestTitle')}>
            <span>{$tr('appTerminals.aiSuggest')}</span>
            <small>{$tr('appTerminals.aiSuggestSub')}</small>
          </button>
          <button type="button" on:click={() => openAIPanel('write_command_from_goal')} class="ai-menu-item" title={$tr('appTerminals.aiWriteTitle')}>
            <span>{$tr('appTerminals.aiWrite')}</span>
            <small>{$tr('appTerminals.aiWriteSub')}</small>
          </button>
          <button type="button" on:click={() => openAIPanel('run_template_tool')} class="ai-menu-item" title={$tr('appTerminals.aiToolTitle')}>
            <span>{$tr('appTerminals.aiTool')}</span>
            <small>{$tr('appTerminals.aiToolSub')}</small>
          </button>
        </div>
      {/if}
    </div>
  </div>
  <div class="controls-right">
    <span class="terminal-count">AI: {aiSettings.provider === 'ollama' ? 'Ollama' : 'OpenAI'}</span>
    <span class="terminal-count">{$tr('appTerminals.active', { n: visibleTerminalCount })}</span>
  </div>
</div>

{#if errorMessage}
  <div class="error-message">
    <span class="error-icon">!</span>
    <span class="error-text">{errorMessage}</span>
    <button class="error-dismiss" on:click={dismissError}>×</button>
  </div>
{/if}

<div class="terminal-and-notes">
  <div class="terminal-area">
    {#if layoutTree}
      <SplitPane
        node={layoutTree}
        {terminalMap}
        {activeTerminalId}
        on:activate={(e) => setActiveTerminalId(e.detail)}
        on:split={(e) => splitTerminal(e.detail.id, e.detail.direction)}
        on:minimize={(e) => toggleMinimize(e.detail)}
        on:close={(e) => removeTerminal(e.detail)}
        on:reconnect={(e) => reconnectSSH(e.detail)}
        on:editname={(e) => startEditingName(e.detail)}
        on:savename={(e) => saveTerminalName(e.detail.id, e.detail.event)}
        on:resize={handleResize}
        on:ratiochange={touchLayoutTree}
        on:dragstart={(e) => handleDragStart(e.detail.event, e.detail.id)}
        on:dragover={(e) => handleDragOver(e.detail.event, e.detail.id)}
        on:dragleave={(e) => handleDragLeave(e.detail.event)}
        on:drop={(e) => handleDrop(e.detail.event, e.detail.id)}
        on:dragend={(e) => handleDragEnd(e.detail.event)}
        on:searchinput={(e) => updateTerminalSearchQuery(e.detail.id, e.detail.query)}
        on:searchnext={(e) => terminalSearchNext(e.detail)}
        on:searchprev={(e) => terminalSearchPrev(e.detail)}
        on:searchclose={(e) => closeTerminalSearch(e.detail)}
        on:dismissrestore={(e) => dismissRestoreSummary(e.detail)}
        on:togglerecording={(e) => toggleRecording(e.detail)}
        on:opentranscript={(e) => openTranscriptViewer(e.detail)}
      />
    {:else}
      <div class="empty-state">
        <p>{$tr('appTerminals.noTerminals')}</p>
      </div>
    {/if}
  </div>
  {#if notesPanelOpen}
    <button
      type="button"
      class="notes-divider"
      aria-label="Resize notes panel"
      on:mousedown={startNotesDrag}
    ></button>
    <div class="notes-panel" style="width:{notesPanelWidth}px">
      <MarkdownNotes
        sshTerminals={terminals.filter(t => t.type === 'ssh' && !t.minimized)}
        on:close={closeNotesPanel}
      />
    </div>
  {/if}
</div>
