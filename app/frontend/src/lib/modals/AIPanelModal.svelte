<script>
  // AI action panel. Owns run/insert logic (calls the backend directly via the
  // Wails runtime bridge); the parent opens it (sets `state`) and supplies the
  // live terminal output. Shared modal styles come from the global stylesheets (styles/).
  import { t } from '../i18n.js';

  const app = () => window['go']['main']['App'];

  export let state;                 // aiPanelState: { mode, terminalId, terminalType, terminalName, goal, result, loading }
  export let terminalOutput = '';   // live output buffer of the active terminal
  export let onClose = () => {};
  export let onError = () => {};

  const knownModes = ['explain_output', 'suggest_next_command', 'write_command_from_goal', 'run_template_tool'];

  function titleKey(mode) {
    return knownModes.includes(mode) ? mode : 'default';
  }

  function needsGoal(mode) {
    return mode === 'write_command_from_goal' || mode === 'run_template_tool';
  }

  $: title = $t(`aiPanel.titles.${titleKey(state.mode)}`);
  $: showGoalInput = needsGoal(state.mode);
  // Insert is offered only for clean, single-command suggestions.
  $: canInsert =
    state &&
    state.result &&
    !state.warning &&
    state.mode !== 'explain_output' &&
    state.mode !== 'run_template_tool';

  async function run() {
    if (needsGoal(state.mode) && !state.goal.trim()) {
      onError($t('aiPanel.errors.goalRequired'));
      return;
    }
    state = { ...state, loading: true, result: '', warning: '' };
    try {
      if (state.mode === 'run_template_tool') {
        const result = await app()['RunAITemplateTool'](state.terminalId, state.goal, state.terminalType, state.terminalName, terminalOutput || '');
        state = { ...state, loading: false, result, warning: '' };
      } else {
        const raw = await app()['AskAI'](state.mode, state.goal, state.terminalType, state.terminalName, terminalOutput || '');
        // AskAI returns JSON { text, warning }; tolerate a plain string too.
        let text = raw;
        let warning = '';
        try {
          const parsed = JSON.parse(raw);
          if (parsed && typeof parsed === 'object') {
            text = parsed.text ?? '';
            warning = parsed.warning ?? '';
          }
        } catch {
          /* not JSON — treat as plain text */
        }
        state = { ...state, loading: false, result: text, warning };
      }
    } catch (error) {
      state = { ...state, loading: false };
      onError($t('aiPanel.errors.requestFailed', { error: error.message || error }));
    }
  }

  async function insert() {
    if (!state.result || !state.result.trim()) {
      return;
    }
    try {
      await app()['WriteToTerminal'](state.terminalId, state.result + '\r');
    } catch (error) {
      onError($t('aiPanel.errors.insertFailed', { error: error.message || error }));
    }
  }
</script>

<div
  class="modal-overlay"
  on:click={onClose}
  on:keydown={(e) => { if (e.key === 'Escape') onClose(); }}
  tabindex="0"
  role="button"
>
  <div
    class="template-prompt-modal"
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    on:click|stopPropagation
    on:keydown|stopPropagation
  >
    <div class="template-prompt-header">
      <h3>{title}</h3>
      <button type="button" class="modal-close-button" on:click={onClose}>&#x2715;</button>
    </div>
    <p class="template-prompt-text">
      {$t('aiPanel.terminalLine', { name: state.terminalName, type: state.terminalType.toUpperCase() })}
    </p>
    {#if showGoalInput}
      <label class="template-prompt-field">
        <span>{$t('aiPanel.goalLabel')}</span>
        <textarea
          bind:value={state.goal}
          placeholder={$t('aiPanel.goalPlaceholder')}
        ></textarea>
      </label>
    {/if}
    {#if state.result}
      <div class="ai-result-block">
        <span class="ai-result-label">{$t('aiPanel.result')}</span>
        <pre>{state.result}</pre>
      </div>
    {/if}
    {#if state.warning}
      <p class="template-prompt-text" style="color: #e3b341;">
        {$t('aiPanel.reviewNote', { warning: state.warning })}
      </p>
    {/if}
    <div class="template-prompt-actions">
      {#if canInsert}
        <button type="button" class="modal-secondary-button" on:click={insert}>{$t('aiPanel.insert')}</button>
      {/if}
      <button type="button" class="modal-secondary-button" on:click={onClose}>{$t('aiPanel.close')}</button>
      <button type="button" class="modal-primary-button" on:click={run} disabled={state.loading}>
        {state.loading ? $t('aiPanel.running') : $t('aiPanel.run')}
      </button>
    </div>
  </div>
</div>
