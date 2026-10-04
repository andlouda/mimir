import { get } from 'svelte/store';
import { terminals, layoutTree, activeTerminalId } from '../stores/terminalStore.js';
import { SaveCurrentSession } from '../../../wailsjs/go/main/App';
import { getTranscriptExcerpt } from '../transcript/transcriptApi.js';
import { sanitizeTranscriptPreview } from '../util.js';
import { serializeLayout } from '../terminals/layoutTree.js';

let sessionSaveTimer = null;

// ---- layout persistence ---------------------------------------------------
// The split tree is saved with stable leaf keys so a restart rebuilds the
// user's arrangement. Persistence starts after the session restore finished
// (enableLayoutPersistence), so the intermediate trees of the restore are
// never written.
let layoutPersistenceOn = false;
let layoutTimer = null;

export function layoutKeyFor(term) {
  if (!term) return null;
  if (term.resumeId) return `r:${term.resumeId}`;
  if (term.tmuxSessionName) return `t:${term.tmuxSessionName}`;
  return `n:${term.type}:${term.name}`;
}

export function persistLayout() {
  const update = window['go']?.['main']?.['App']?.['UpdateSessionLayout'];
  if (typeof update !== 'function') return;
  const list = get(terminals);
  const saved = serializeLayout(get(layoutTree), (id) => layoutKeyFor(list.find((t) => t.id === id)));
  Promise.resolve(update(saved ? JSON.stringify(saved) : '')).catch((error) => console.error('Failed to store layout:', error));
  scheduleSessionSave();
}

export function enableLayoutPersistence() {
  if (layoutPersistenceOn) return;
  layoutPersistenceOn = true;
  layoutTree.subscribe(() => {
    if (layoutTimer) clearTimeout(layoutTimer);
    layoutTimer = setTimeout(() => { layoutTimer = null; persistLayout(); }, 300);
  });
}

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
