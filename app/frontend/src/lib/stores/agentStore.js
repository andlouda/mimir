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

agentDetectionEnabled.subscribe((enabled) => {
  try {
    localStorage.setItem(DETECTION_KEY, enabled ? 'on' : 'off');
  } catch {
    /* localStorage unavailable */
  }
});
