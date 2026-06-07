import { get } from 'svelte/store';
import { terminals, layoutTree, activeTerminalId } from '../stores/terminalStore.js';
import { SaveCurrentSession } from '../../../wailsjs/go/main/App';
import { getTranscriptExcerpt } from '../transcript/transcriptApi.js';
import { sanitizeTranscriptPreview } from '../util.js';

let sessionSaveTimer = null;

export function scheduleSessionSave(delayMs = 250) {
  if (sessionSaveTimer) {
    clearTimeout(sessionSaveTimer);
  }
  sessionSaveTimer = setTimeout(() => {
    SaveCurrentSession().catch((error) => {
      console.error('Failed to save current session:', error);
    });
    sessionSaveTimer = null;
  }, delayMs);
}

export function clearSessionSaveTimer() {
  if (sessionSaveTimer) {
    clearTimeout(sessionSaveTimer);
    sessionSaveTimer = null;
  }
}

export function persistTerminalState(term, overrides = {}) {
  const next = { ...term, ...overrides };
  window['go']['main']['App']['UpdateTerminalState'](
    next.id,
    next.type,
    next.name,
    next.minimized,
    next.sshProfileId || '',
    next.tmuxSessionName || '',
    next.resumeId || '',
    next.restoreClass || 'fresh',
    next.folderId || ''
  );
  scheduleSessionSave();
}

export async function loadTranscriptExcerpt(resumeId, maxBytes = 8000) {
  if (!resumeId) return '';
  try {
    const raw = await getTranscriptExcerpt(resumeId, maxBytes);
    return sanitizeTranscriptPreview(raw);
  } catch (error) {
    console.error(`Failed to load transcript excerpt for ${resumeId}:`, error);
    return '';
  }
}
