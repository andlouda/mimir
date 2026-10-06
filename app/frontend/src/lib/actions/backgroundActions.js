import { get } from 'svelte/store';
import { terminals } from '../stores/terminalStore.js';
import { normalizeBackground, serializeBackground } from '../terminals/background.js';
import { scheduleSessionSave } from './sessionActions.js';
import { XTERM_THEME } from './terminalActions.js';
import { terminalRenderer } from '../stores/uiStore.js';
import { WebglAddon } from '@xterm/addon-webgl';
import { disableWebgl, enableWebgl } from '../terminals/xtermLifecycle.js';

// Transparent xterm background so the image layer behind it shows through;
// everything else keeps the shared theme.
const TRANSPARENT_THEME = { ...XTERM_THEME, background: 'rgba(12, 14, 20, 0)' };

function callBackend(name, ...args) {
  const fn = window['go']?.['main']?.['App']?.[name];
  if (typeof fn !== 'function') return Promise.resolve(undefined);
  return Promise.resolve(fn(...args));
}

/** Sets (or clears, with null) one pane's background and persists it. */
export function applyTerminalBackground(terminalId, background) {
  const bg = normalizeBackground(background);
  let found = null;
  terminals.update((list) => list.map((t) => {
    if (t.id !== terminalId) return t;
    found = t;
    return { ...t, background: bg };
  }));
  if (!found) return null;
  const xterm = found.terminal;
  if (xterm?.options) {
    try {
      const wantTransparent = !!bg;
      const changed = !!xterm.options.allowTransparency !== wantTransparent;
      // Transparency only while a picture is behind the pane: it costs the
      // WebGL renderer a slower glyph path and a different compositing
      // route, so it is not left on for plain panes.
      xterm.options.allowTransparency = wantTransparent;
      xterm.options.theme = wantTransparent ? TRANSPARENT_THEME : XTERM_THEME;
      if (changed && found.webgl) {
        disableWebgl(found);
        if (get(terminalRenderer) === 'auto') enableWebgl(found, WebglAddon);
      }
    } catch { /* disposed */ }
  }
  callBackend('UpdateTerminalBackground', terminalId, serializeBackground(bg)).catch((error) => console.error('Failed to store background:', error));
  scheduleSessionSave();
  return bg;
}

export function applyBackgroundToAll(background) {
  for (const t of get(terminals)) applyTerminalBackground(t.id, background);
}

/** Native file dialog → imported record { id, name, size, addedAt } or null. */
export async function importBackgroundImage() {
  const json = await callBackend('ImportTerminalBackground');
  return json ? JSON.parse(json) : null;
}

export async function listBackgroundImages() {
  try {
    const json = await callBackend('ListTerminalBackgroundsJSON');
    const list = json ? JSON.parse(json) : [];
    return Array.isArray(list) ? list : [];
  } catch {
    return [];
  }
}

/** Deletes the file and clears it from every pane that used it. */
export async function deleteBackgroundImage(id) {
  await callBackend('DeleteTerminalBackground', id);
  for (const t of get(terminals)) {
    if (t.background?.id === id) applyTerminalBackground(t.id, null);
  }
}
