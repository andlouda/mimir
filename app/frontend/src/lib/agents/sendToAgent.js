import { get } from 'svelte/store';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
import { agentStates } from '../stores/agentStore.js';

// "Send to agent": hand a piece of text (a terminal selection, a note, a
// path) to the pane an agent runs in. The text is pasted through xterm's
// bracketed paste, so the agent receives it as typed input and the user
// still presses Enter — nothing is submitted on their behalf.

/** Panes with a running agent, excluding `excludeId`. */
export function agentTargets(excludeId = null) {
  const states = get(agentStates);
  return get(terminals)
    .filter((t) => states[t.id] && t.id !== excludeId)
    .map((t) => ({ id: t.id, name: t.name, label: states[t.id].short || states[t.id].label || 'Agent', kind: states[t.id].kind }));
}

/** Pastes text into the agent's pane and brings that pane to the front. */
export function sendTextToAgent(targetId, text) {
  const target = get(terminals).find((t) => t.id === targetId);
  const xterm = target?.terminal;
  const body = String(text || '').replace(/\n+$/, '');
  if (!target || !body.trim() || typeof xterm?.paste !== 'function') return false;
  if (target.minimized) return false;
  xterm.paste(body);
  activeTerminalId.set(targetId);
  try { xterm.focus?.(); } catch { /* ignore */ }
  return true;
}
