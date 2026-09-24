import { writable } from 'svelte/store';

// Per-terminal agent state, keyed by terminal id:
// {
//   kind, label, pid, cwd, source, transcripts   ← from backend detection
//   status: 'working' | 'idle' | 'unknown'        ← from terminal title changes
//   subject: string                               ← what the agent is doing (title text)
//   attention: boolean                            ← finished while the pane was not active
//   detectedAt: number
// }
export const agentStates = writable({});

// Terminal id whose agent transcript panel is open, or null.
export const agentPanelTerminalId = writable(null);

// When false (default) the open panel follows the terminal the user
// selects; when true it stays on the terminal it was pinned to.
export const agentPanelPinned = writable(false);

const DETECTION_KEY = 'mimir-agent-detection';

function readDetectionSetting() {
  try {
    const saved = localStorage.getItem(DETECTION_KEY);
    if (saved === 'off') return false;
  } catch {
    /* localStorage unavailable */
  }
  return true;
}

// Whether Mimir looks for coding agents in terminals (on by default; the
// probe only inspects the pane's process list out-of-band).
export const agentDetectionEnabled = writable(readDetectionSetting());

let syncingFromBackend = false;

agentDetectionEnabled.subscribe((enabled) => {
  try {
    localStorage.setItem(DETECTION_KEY, enabled ? 'on' : 'off');
  } catch {
    /* localStorage unavailable */
  }
  // The backend keeps the authoritative copy: it also gates the prompt hook
  // that reports the working directory of terminals without tmux.
  if (syncingFromBackend) return;
  const setter = globalThis.window?.['go']?.['main']?.['App']?.['SetAgentDetectionEnabled'];
  if (typeof setter === 'function') {
    Promise.resolve(setter(enabled)).catch((error) => console.error('Could not save agent detection setting:', error));
  }
});

const NOTIFY_KEY = 'mimir-agent-notify';

function readNotifySetting() {
  try {
    if (localStorage.getItem(NOTIFY_KEY) === 'off') return false;
  } catch {
    /* localStorage unavailable */
  }
  return true;
}

// Desktop notification when an agent finishes or needs an approval while
// its pane is not in front (on by default; a per-machine UI preference).
export const agentNotificationsEnabled = writable(readNotifySetting());
agentNotificationsEnabled.subscribe((enabled) => {
  try {
    localStorage.setItem(NOTIFY_KEY, enabled ? 'on' : 'off');
  } catch {
    /* localStorage unavailable */
  }
});

// Persistent notes on agent sessions and projects, keyed by host + session
// file / host + git root (see agentActions.sessionKey / projectKey). Loaded
// from the backend once; writes go through agentActions.
export const agentAnnotations = writable({ sessions: {}, projects: {} });

export async function loadAgentAnnotations() {
  const getter = globalThis.window?.['go']?.['main']?.['App']?.['GetAgentAnnotationsJSON'];
  if (typeof getter !== 'function') return;
  try {
    const parsed = JSON.parse(await getter());
    agentAnnotations.set({ sessions: parsed?.sessions || {}, projects: parsed?.projects || {} });
  } catch (error) {
    console.warn('Could not load agent annotations:', error);
  }
}

/** Loads the persisted setting from the backend (call once at start-up). */
export async function loadAgentDetectionSetting() {
  const getter = globalThis.window?.['go']?.['main']?.['App']?.['IsAgentDetectionEnabled'];
  if (typeof getter !== 'function') return;
  try {
    const enabled = await getter();
    syncingFromBackend = true;
    agentDetectionEnabled.set(Boolean(enabled));
  } catch (error) {
    console.warn('Could not load agent detection setting:', error);
  } finally {
    syncingFromBackend = false;
  }
}
