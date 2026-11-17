// Settings of the local masking model (see app/ai_term_detector.go): a model
// on this machine or a private network that names host, account and other
// names so Mimir can hide them before text goes to a cloud model or an
// agent's pane.
import { optional } from '../backend.js';

export const DLP_DEFAULTS = Object.freeze({ localModelEnabled: false, baseUrl: '', model: '' });

function normalize(raw) {
  return {
    localModelEnabled: Boolean(raw?.localModelEnabled),
    baseUrl: typeof raw?.baseUrl === 'string' ? raw.baseUrl : '',
    model: typeof raw?.model === 'string' ? raw.model : '',
  };
}

/** Current settings; the defaults (stage off) when the backend has none. */
export async function loadDLPSettings({ load = optional('GetDLPSettingsJSON') } = {}) {
  if (typeof load !== 'function') return { ...DLP_DEFAULTS };
  return normalize(JSON.parse((await load()) || '{}'));
}

/**
 * Saves a change and returns the stored settings. Rejects with the backend's
 * reason, for instance when the endpoint is not on a private network.
 */
export async function saveDLPSettings(current, patch, { save = optional('UpdateDLPSettingsJSON') } = {}) {
  if (typeof save !== 'function') throw new Error('This Mimir version has no local masking model.');
  return normalize(JSON.parse(await save(JSON.stringify({ ...normalize(current), ...patch }))));
}
