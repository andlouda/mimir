import { get } from 'svelte/store';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
import { agentStates } from '../stores/agentStore.js';
import { optional } from '../backend.js';

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

// The masked bridge (see app/app_agent_mask.go): towards the agent, secrets
// are scrubbed and IP/MAC/e-mail addresses become placeholders such as
// MIMIR_IP_1; back from the agent, a selected command gets the real values
// again before it is pasted into another pane. The mapping belongs to the
// agent's pane and lives in the backend.

/**
 * Masks text and pastes it into the agent's pane. Fails closed: without a
 * backend that can mask, nothing is sent.
 * @returns {Promise<{ ok: boolean, masked: Record<string, number> }>}
 */
export async function sendMaskedTextToAgent(targetId, text, { mask = optional('MaskTextForAgentJSON') } = {}) {
  const body = String(text || '');
  if (!body.trim() || typeof mask !== 'function') return { ok: false, masked: {} };
  const result = JSON.parse(await mask(targetId, body));
  if (typeof result?.text !== 'string') return { ok: false, masked: {} };
  return { ok: sendTextToAgent(targetId, result.text), masked: result.masked || {} };
}

/** Panes a command from the agent in `agentId` can be pasted into. */
export function pasteTargets(agentId) {
  return get(terminals)
    .filter((t) => t.id !== agentId && !t.minimized)
    .map((t) => ({ id: t.id, name: t.name }));
}

/**
 * Translates text selected in the agent's pane back to the real values and
 * pastes it into another pane (bracketed paste, no Enter). A text that still
 * holds a placeholder the mapping does not know is not pasted: there is no
 * real value behind it.
 * @returns {Promise<{ ok: boolean, restored: number, unknown: string[] }>}
 */
export async function pasteUnmaskedFromAgent(agentId, targetId, text, { unmask = optional('UnmaskTextFromAgentJSON') } = {}) {
  const body = String(text || '').replace(/\n+$/, '');
  if (!body.trim() || typeof unmask !== 'function') return { ok: false, restored: 0, unknown: [] };
  const result = JSON.parse(await unmask(agentId, body));
  const unknown = Array.isArray(result?.unknown) ? result.unknown : [];
  if (unknown.length > 0 || typeof result?.text !== 'string') return { ok: false, restored: 0, unknown };
  const target = get(terminals).find((t) => t.id === targetId);
  const xterm = target?.terminal;
  if (!target || target.minimized || typeof xterm?.paste !== 'function') return { ok: false, restored: 0, unknown };
  xterm.paste(result.text);
  activeTerminalId.set(targetId);
  try { xterm.focus?.(); } catch { /* ignore */ }
  return { ok: true, restored: result.restored || 0, unknown };
}
