import { get } from 'svelte/store';
import { agentDetectionEnabled, agentPanelTerminalId, agentStates } from '../stores/agentStore.js';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
import { outputMentionsAgent, parseAgentTitle } from '../agents/agentSignals.js';

function app() {
  return window['go']['main']['App'];
}

// Detection is event-driven: window titles (Claude Code), startup banners
// in the output (Codex, Gemini, Aider, OpenCode) and the shell's prompt
// beacon (program returned to the shell) trigger a probe. There is no
// periodic polling of idle terminals. While an agent is known, a slow
// liveness check notices agents that exit without a prompt beacon (the
// shell hook is opt-in), so a stale badge disappears within a few minutes.
const LIVENESS_MS = 3 * 60 * 1000;
const DEBOUNCE_MS = 1500;
const INITIAL_DELAY_MS = 2500;

const watches = new Map(); // terminalId → { type, timer, pending, lastRun }

function terminalType(id) {
  const watch = watches.get(id);
  if (watch?.type) return watch.type;
  return get(terminals).find((t) => t.id === id)?.type || '';
}

function setState(id, patch) {
  agentStates.update((states) => {
    const current = states[id];
    if (patch === null) {
      if (!current) return states;
      const next = { ...states };
      delete next[id];
      return next;
    }
    return { ...states, [id]: { ...(current || {}), ...patch } };
  });
}

/**
 * Runs backend detection for a terminal. Debounced per terminal so bursts of
 * title changes do not translate into bursts of process probes.
 */
export function scheduleAgentDetection(id, { immediate = false } = {}) {
  if (!get(agentDetectionEnabled)) return;
  const watch = watches.get(id) || { type: terminalType(id), timer: null, pending: null, lastRun: 0 };
  watches.set(id, watch);
  if (watch.pending) return;
  const wait = immediate ? 0 : Math.max(0, DEBOUNCE_MS - (Date.now() - watch.lastRun));
  watch.pending = setTimeout(() => {
    watch.pending = null;
    runAgentDetection(id).catch((error) => console.error(`Agent detection failed for terminal ${id}:`, error));
  }, wait);
}

export async function runAgentDetection(id) {
  const watch = watches.get(id);
  if (watch) watch.lastRun = Date.now();
  if (!get(agentDetectionEnabled)) return null;
  const type = terminalType(id);
  const raw = await app()['DetectAgentForTerminalJSON'](id, type);
  const result = JSON.parse(raw || '{}');
  if (!result.detected) {
    const had = get(agentStates)[id];
    if (had) {
      setState(id, null);
      if (get(agentPanelTerminalId) === id) agentPanelTerminalId.set(null);
    }
    setLiveness(id, false);
    return result;
  }
  setLiveness(id, true);
  const previous = get(agentStates)[id];
  const unchanged = previous
    && previous.kind === result.kind
    && previous.pid === result.pid
    && previous.cwd === result.cwd
    && previous.source === result.source
    && previous.tmux === !!result.tmux;
  if (unchanged) return result;
  setState(id, {
    kind: result.kind,
    label: result.label,
    pid: result.pid,
    cwd: result.cwd,
    source: result.source,
    transcripts: !!result.transcripts,
    tmux: !!result.tmux,
    status: previous?.status || 'unknown',
    subject: previous?.subject || '',
    attention: previous?.attention || false,
    detectedAt: previous?.kind === result.kind ? previous.detectedAt : Date.now(),
  });
  return result;
}

function setLiveness(id, on) {
  const watch = watches.get(id);
  if (!watch) return;
  if (!on) {
    if (watch.timer) clearInterval(watch.timer);
    watch.timer = null;
    return;
  }
  if (!watch.timer) watch.timer = setInterval(() => scheduleAgentDetection(id), LIVENESS_MS);
}

/** The shell showed a new prompt: a running agent has most likely exited. */
export function handleTerminalPrompt(id) {
  if (!get(agentDetectionEnabled)) return;
  if (get(agentStates)[id]) scheduleAgentDetection(id);
}

/** Feeds an xterm title change into the agent state machine. */
export function handleTerminalTitle(id, title) {
  if (!get(agentDetectionEnabled)) return;
  const parsed = parseAgentTitle(title);
  const current = get(agentStates)[id];
  if (!parsed.agentLike) {
    // A cleared or ordinary title while an agent was known usually means it
    // exited; re-check instead of guessing.
    if (current) scheduleAgentDetection(id);
    return;
  }
  if (!current) {
    // Agent-like title but nothing detected yet: probe now, keep the hint so
    // the badge can show state as soon as detection confirms the kind.
    scheduleAgentDetection(id, { immediate: true });
  }
  const finished = current?.status === 'working' && parsed.status === 'idle';
  const attention = finished && get(activeTerminalId) !== id ? true : (current?.attention || false);
  // Claude Code re-sets its title about once a second while working (the
  // spinner glyph rotates). Only write to the store when the visible state
  // actually changes; every store write re-renders all pane headers.
  if (current && current.status === parsed.status && current.subject === parsed.subject && current.attention === attention) return;
  setState(id, { status: parsed.status, subject: parsed.subject, attention, lastChange: Date.now() });
}

/** Cheap sniff of terminal output for agent startup banners. */
export function noteTerminalOutput(id, chunk) {
  if (!get(agentDetectionEnabled)) return;
  if (get(agentStates)[id]) return;
  if (outputMentionsAgent(chunk)) scheduleAgentDetection(id);
}

/** Starts periodic detection for a terminal (call once after it is ready). */
export function startAgentWatch(id, type) {
  stopAgentWatch(id);
  const watch = { type, timer: null, pending: null, lastRun: 0 };
  watches.set(id, watch);
  if (!get(agentDetectionEnabled)) return;
  // One probe after start-up covers restored sessions where an agent is
  // already running; afterwards only events trigger probes.
  setTimeout(() => scheduleAgentDetection(id, { immediate: true }), INITIAL_DELAY_MS);
}

export function stopAgentWatch(id) {
  const watch = watches.get(id);
  if (!watch) return;
  if (watch.timer) clearInterval(watch.timer);
  if (watch.pending) clearTimeout(watch.pending);
  watches.delete(id);
  setState(id, null);
  if (get(agentPanelTerminalId) === id) agentPanelTerminalId.set(null);
}

export function markAgentAttentionSeen(id) {
  const current = get(agentStates)[id];
  if (current?.attention) setState(id, { attention: false });
}

export function openAgentPanel(id) {
  markAgentAttentionSeen(id);
  agentPanelTerminalId.set(id);
}

export function closeAgentPanel() {
  agentPanelTerminalId.set(null);
}

export function toggleAgentPanel(id) {
  if (get(agentPanelTerminalId) === id) closeAgentPanel();
  else openAgentPanel(id);
}

/** Loads the agent transcript for a terminal from the backend. */
export async function loadAgentTranscript(id, limit = 12) {
  const raw = await app()['GetAgentTranscriptJSON'](id, terminalType(id), limit);
  return JSON.parse(raw);
}

// Looking at a pane clears its "finished while you were away" marker.
activeTerminalId.subscribe((id) => {
  if (id != null) markAgentAttentionSeen(id);
});

// Turning detection off drops all state and timers; turning it on re-probes.
agentDetectionEnabled.subscribe((enabled) => {
  if (enabled) {
    for (const id of watches.keys()) scheduleAgentDetection(id, { immediate: true });
    return;
  }
  agentStates.set({});
  agentPanelTerminalId.set(null);
});

/** Loads the raw tmux pane text (screen + scrollback) for a terminal. */
export async function loadAgentPaneText(id, { full = false } = {}) {
  const raw = await app()['GetAgentPaneTextJSON'](id, terminalType(id), full);
  return JSON.parse(raw);
}

/** Loads git status / diff stat of the agent's working directory. */
export async function loadAgentGitStatus(id) {
  const raw = await app()['GetAgentGitStatusJSON'](id, terminalType(id));
  return JSON.parse(raw);
}

/** Test hook: number of active watches. */
export function _activeWatchCount() {
  return watches.size;
}
