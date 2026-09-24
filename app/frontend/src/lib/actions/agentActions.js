import { get } from 'svelte/store';
import { agentAnnotations, agentDetectionEnabled, agentNotificationsEnabled, agentPanelTerminalId, agentStates } from '../stores/agentStore.js';
import { t } from '../i18n.js';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
import { outputMentionsAgent, parseAgentTitle } from '../agents/agentSignals.js';
import { EventsOn } from '../../../wailsjs/runtime';

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

const watches = new Map(); // terminalId → { type, timer, pending, lastRun, offState }

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
    unsubscribeState(id);
    return result;
  }
  setLiveness(id, true);
  if (result.transcripts) subscribeState(id);
  const previous = get(agentStates)[id];
  const unchanged = previous
    && previous.kind === result.kind
    && previous.pid === result.pid
    && previous.cwd === result.cwd
    && previous.source === result.source
    && previous.tmux === !!result.tmux;
  if (unchanged) return result;
  const keepProject = previous?.project && previous.cwd === result.cwd && previous.source === result.source;
  setState(id, {
    kind: result.kind,
    project: keepProject ? previous.project : null,
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
  if (!keepProject && result.cwd) loadAgentProject(id);
  return result;
}

/**
 * Project of an agent: host plus git root (and branch / worktree) of its
 * working directory. Looked up once per detection with a new directory.
 */
export async function loadAgentProject(id) {
  try {
    const raw = await app()['GetAgentProjectJSON'](id, terminalType(id));
    const project = JSON.parse(raw);
    if (get(agentStates)[id]) setState(id, { project });
    return project;
  } catch (error) {
    console.warn('Could not resolve agent project:', error);
    return null;
  }
}

/** Host part of the annotation keys: "local", "wsl" or the SSH host. */
function agentHost(agent) {
  return agent?.project?.host || agent?.source || 'local';
}

export function sessionKey(agent) {
  if (!agent?.sessionFile) return '';
  return `${agentHost(agent)}|${agent.sessionFile}`;
}

export function projectKey(agent) {
  const root = agent?.project?.root || agent?.cwd || '';
  if (!root) return '';
  return `${agentHost(agent)}|${root}`;
}

/** Name / note / archived flag of a session, persisted by the backend. */
export async function setSessionAnnotation(id, { name = '', note = '', archived = false } = {}) {
  const agent = get(agentStates)[id];
  const key = sessionKey(agent);
  if (!key) return false;
  await app()['SetAgentSessionAnnotation'](key, name, note, Boolean(archived));
  agentAnnotations.update((all) => {
    const sessions = { ...all.sessions };
    if (!name && !note && !archived) delete sessions[key];
    else sessions[key] = { name, note, archived: Boolean(archived) };
    return { ...all, sessions };
  });
  return true;
}

/** Display name of a project (persisted by the backend). */
export async function setProjectName(id, name) {
  const agent = get(agentStates)[id];
  const key = projectKey(agent);
  if (!key) return false;
  await app()['SetAgentProjectName'](key, name || '');
  agentAnnotations.update((all) => {
    const projects = { ...all.projects };
    if (!name) delete projects[key];
    else projects[key] = { name };
    return { ...all, projects };
  });
  return true;
}

/** Fallback project label: the git root's (or cwd's) last path segment. */
export function projectFolderName(agent) {
  const root = agent?.project?.root || agent?.cwd || '';
  const parts = root.split(/[\\/]/).filter(Boolean);
  return parts.length ? parts[parts.length - 1] : '';
}

// The backend follows the agent's session file and emits
// "agent-state-<id>" whenever the derived state changes. That is the
// source of truth for working / waiting; window titles only fill in while
// no file event has arrived yet.
function subscribeState(id) {
  const watch = watches.get(id);
  if (!watch || watch.offState) return;
  watch.offState = EventsOn(`agent-state-${id}`, (raw) => handleAgentStateEvent(id, raw));
}

function unsubscribeState(id) {
  const watch = watches.get(id);
  if (watch?.offState) {
    try { watch.offState(); } catch { /* already gone */ }
    watch.offState = null;
  }
}

export function handleAgentStateEvent(id, raw) {
  let payload = raw;
  if (typeof raw === 'string') {
    try { payload = JSON.parse(raw); } catch { return; }
  }
  if (!payload || typeof payload !== 'object') return;
  const current = get(agentStates)[id];
  if (!current) return;
  const status = payload.state === 'working' || payload.state === 'idle' || payload.state === 'permission' ? payload.state : 'unknown';
  const lastText = String(payload.lastText || '');
  const subject = lastText.split('\n').find((l) => l.trim()) || current.subject || '';
  // Attention: the agent finished, or (via the approval hook) is blocked on
  // a permission prompt — both while the user looks at another pane.
  const finished = current.status === 'working' && status === 'idle';
  const blocked = status === 'permission' && current.status !== 'permission';
  const attention = (finished || blocked) && get(activeTerminalId) !== id ? true : (current.attention || false);
  if (blocked) notifyAgentEvent(id, 'permission', lastText);
  else if (finished) notifyAgentEvent(id, 'done', '');
  const prompt = status === 'permission' || status === 'idle' ? String(payload.prompt || '') : '';
  const activity = status === 'working' ? String(payload.activity || '') : '';
  const activityAt = activity ? String(payload.activityAt || '') : '';
  const title = String(payload.title || current.title || '');
  if (current.fileState && current.status === status && current.lastText === lastText && current.attention === attention && current.prompt === prompt && current.activity === activity && current.title === title) return;
  setState(id, { status, subject: subject.length > 120 ? subject.slice(0, 120) + '…' : subject, lastText, lastAt: payload.lastAt || '', sessionFile: payload.sessionFile || current.sessionFile || '', attention, prompt, activity, activityAt, title, answering: false, fileState: true, lastChange: Date.now() });
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
  // Once the session file drives the state, titles are only noise.
  if (current?.fileState) return;
  const finished = current?.status === 'working' && parsed.status === 'idle';
  const attention = finished && get(activeTerminalId) !== id ? true : (current?.attention || false);
  if (finished) notifyAgentEvent(id, 'done', '');
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
  unsubscribeState(id);
  watches.delete(id);
  setState(id, null);
  if (get(agentPanelTerminalId) === id) agentPanelTerminalId.set(null);
}

function windowInFront() {
  try {
    return typeof document !== 'undefined' && typeof document.hasFocus === 'function' ? document.hasFocus() : true;
  } catch {
    return true;
  }
}

/**
 * Desktop notification for an agent event when the user is not looking at
 * that pane: another pane is active, or the Mimir window is not focused.
 * Text is metadata only (agent label, terminal name, the hook's one-line
 * message); the backend collapses bursts per terminal.
 */
export function notifyAgentEvent(id, kind, message) {
  if (!get(agentNotificationsEnabled)) return false;
  if (get(activeTerminalId) === id && windowInFront()) return false;
  const agent = get(agentStates)[id];
  if (!agent) return false;
  const fn = globalThis.window?.['go']?.['main']?.['App']?.['NotifyDesktop'];
  if (typeof fn !== 'function') return false;
  const translate = get(t);
  const termName = get(terminals).find((x) => x.id === id)?.name || `#${id}`;
  const title = `${agent.label} · ${termName}`;
  const body = kind === 'permission'
    ? (message || translate('agentPanel.notifyPermission'))
    : translate('agentPanel.notifyDone');
  Promise.resolve(fn(id, title, body)).catch((error) => console.warn('Desktop notification failed:', error));
  return true;
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

/** Lists candidate sessions for the agent in a terminal (+ current pin). */
export async function loadAgentSessions(id) {
  const raw = await app()['ListAgentSessionsJSON'](id, terminalType(id));
  return JSON.parse(raw || '{}');
}

/** Pins a session for a terminal ('' = automatic). */
export async function selectAgentSession(id, file) {
  await app()['SelectAgentSession'](id, terminalType(id), file || '');
}

/** Loads the process tree below the agent (what runs on the machine right now). */
export async function loadAgentProcesses(id) {
  const raw = await app()['GetAgentProcessesJSON'](id, terminalType(id));
  return JSON.parse(raw);
}

/**
 * "12s" / "3m 05s" since an ISO timestamp; empty when unknown. Used next to
 * the live activity so a hanging command is visible as such.
 */
export function elapsedSince(iso, now = Date.now()) {
  const t = Date.parse(iso || '');
  if (!Number.isFinite(t)) return '';
  const s = Math.max(0, Math.round((now - t) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${String(s % 60).padStart(2, '0')}s`;
  return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, '0')}m`;
}

/** Loads git status / diff stat of the agent's working directory. */
export async function loadAgentGitStatus(id) {
  const raw = await app()['GetAgentGitStatusJSON'](id, terminalType(id));
  return JSON.parse(raw);
}

/**
 * Answers a pending permission prompt (reported by the hook) by typing the
 * key into the agent's pane. The backend refuses when nothing is pending.
 */
export async function answerAgentPermission(id, allow) {
  const current = get(agentStates)[id];
  if (!current || current.status !== 'permission' || current.prompt !== 'permission_prompt' || current.answering) return false;
  setState(id, { answering: true });
  try {
    await app()['AnswerAgentPermission'](id, Boolean(allow));
    return true;
  } catch (error) {
    console.error('Could not answer the permission prompt:', error);
    setState(id, { answering: false });
    return false;
  }
}

/**
 * Claude Code approval hook (Notification hook in ~/.claude/settings.json).
 * terminalId 0 = this machine; otherwise the host of that terminal's agent.
 */
export async function loadClaudeHookStatus(terminalId = 0) {
  const raw = await app()['GetClaudeHookStatusJSON'](terminalId);
  try { return JSON.parse(raw); } catch { return { installed: false, error: 'invalid status' }; }
}

export async function setClaudeHookInstalled(terminalId, installed) {
  await app()[installed ? 'InstallClaudeHook' : 'RemoveClaudeHook'](terminalId);
  return loadClaudeHookStatus(terminalId);
}

/**
 * Settings view: hosts the hook can be installed on from here ("local", and
 * "wsl" on Windows with a distro), each with its status. Claude Code inside
 * WSL reads the distro's own settings.json, so it is a separate host.
 */
export async function loadClaudeHookHosts() {
  let hosts = ['local'];
  try { hosts = JSON.parse(await app()['ListClaudeHookHostsJSON']()) || hosts; } catch { /* keep local */ }
  return Promise.all(hosts.map(async (host) => {
    try {
      const status = JSON.parse(await app()['GetClaudeHookStatusForHostJSON'](host));
      return { host, ...status };
    } catch (e) {
      return { host, installed: false, error: String(e?.message || e) };
    }
  }));
}

export async function setClaudeHookInstalledOnHost(host, installed) {
  await app()[installed ? 'InstallClaudeHookOnHost' : 'RemoveClaudeHookOnHost'](host);
  return JSON.parse(await app()['GetClaudeHookStatusForHostJSON'](host));
}

/** Test hook: number of active watches. */
export function _activeWatchCount() {
  return watches.size;
}
