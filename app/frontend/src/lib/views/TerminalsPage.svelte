<script>
  import { t as tr } from '../i18n.js';
  import SplitPane from '../SplitPane.svelte';
  import MarkdownNotes from '../MarkdownNotes.svelte';
  import AgentTranscriptPanel from '../AgentTranscriptPanel.svelte';
  // Everything this page shows lives in stores and everything it does is an
  // action, so nothing is threaded through App.svelte and AppMainContent.
  import { agentPanelTerminalId } from '../stores/agentStore.js';
  import { aiSettings } from '../stores/aiStore.js';
  import { activeTerminalId, availableTerminalTypes, layoutTree, selectedTerminalType, terminalMap, terminals, visibleTerminalCount } from '../stores/terminalStore.js';
  import { errorMessage, notesPanelOpen, notesPanelWidth, showAIMenu } from '../stores/uiStore.js';
  import { toggleAgentPanel } from '../actions/agentActions.js';
  import { openAIPanel, toggleAIMenu } from '../actions/aiSettingsActions.js';
  import { createDragDropHandlers } from '../actions/dragDrop.js';
  import { closeNotesPanel, startNotesDrag } from '../actions/notesPanelActions.js';
  import { addTerminal, handleResize, reconnectSSH, reinitializeTerminals, removeTerminal, saveTerminalName, splitTerminal, startEditingName, toggleMinimize, toggleRecording } from '../actions/terminalActions.js';
  import { closeTerminalSearch, dismissRestoreSummary, terminalSearchNext, terminalSearchPrev, updateTerminalSearchQuery } from '../actions/terminalSearchActions.js';
  import { persistDefaultTerminalType } from '../actions/terminalTypeActions.js';
  import { openDotEnvViewer, openTranscriptViewer } from '../actions/viewerActions.js';

  const { handleDragStart, handleDragOver, handleDragLeave, handleDrop, handleDragEnd } = createDragDropHandlers({ reinitializeTerminals });
  const dismissError = () => errorMessage.set('');
  const setActiveTerminalId = (id) => activeTerminalId.set(id);
  // A split ratio change mutates the tree in place; nudge the subscribers.
  const touchLayoutTree = () => layoutTree.update((tree) => tree);
</script>

<div class="terminal-controls">
  <div class="controls-left">
    <select bind:value={$selectedTerminalType} on:change={persistDefaultTerminalType} title={$tr('appTerminals.defaultTerminalType')}>
      {#each $availableTerminalTypes as terminalTypeOption (terminalTypeOption.value)}
        <option value={terminalTypeOption.value}>{terminalTypeOption.label}</option>
      {/each}
    </select>
    <button on:click={addTerminal} class="add-btn">{$tr('appTerminals.newTerminal')}</button>
    <div class="ai-menu-wrap">
      <button type="button" on:click={toggleAIMenu} class="ai-btn">AI</button>
      {#if $showAIMenu}
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
    <span class="terminal-count">AI: {$aiSettings.provider === 'ollama' ? 'Ollama' : 'OpenAI'}</span>
    <span class="terminal-count">{$tr('appTerminals.active', { n: $visibleTerminalCount })}</span>
  </div>
</div>


<div class="terminal-and-notes">
  <div class="terminal-area">
    {#if $layoutTree}
      <SplitPane
        node={$layoutTree}
        terminalMap={$terminalMap}
        activeTerminalId={$activeTerminalId}
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
        on:drop={(e) => handleDrop(e.detail.event, e.detail.id, e.detail.zone)}
        on:dragend={(e) => handleDragEnd(e.detail.event)}
        on:searchinput={(e) => updateTerminalSearchQuery(e.detail.id, e.detail.query)}
        on:searchnext={(e) => terminalSearchNext(e.detail)}
        on:searchprev={(e) => terminalSearchPrev(e.detail)}
        on:searchclose={(e) => closeTerminalSearch(e.detail)}
        on:dismissrestore={(e) => dismissRestoreSummary(e.detail)}
        on:togglerecording={(e) => toggleRecording(e.detail)}
        on:opentranscript={(e) => openTranscriptViewer(e.detail)}
        on:openenv={(e) => openDotEnvViewer(e.detail)}
        on:openagent={(e) => toggleAgentPanel(e.detail)}
      />
    {:else}
      <div class="empty-state">
        <p>{$tr('appTerminals.noTerminals')}</p>
      </div>
    {/if}
  </div>
  {#if $agentPanelTerminalId != null}
    <div class="agent-panel">
      <AgentTranscriptPanel terminalId={$agentPanelTerminalId} />
    </div>
  {/if}
  {#if $notesPanelOpen}
    <button
      type="button"
      class="notes-divider"
      aria-label="Resize notes panel"
      on:mousedown={startNotesDrag}
    ></button>
    <div class="notes-panel" style="width:min({$notesPanelWidth}px, calc(100vw - 560px))">
      <MarkdownNotes
        sshTerminals={$terminals.filter(t => t.type === 'ssh' && !t.minimized)}
        on:close={closeNotesPanel}
      />
    </div>
  {/if}
</div>
