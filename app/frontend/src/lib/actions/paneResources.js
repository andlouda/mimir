// CPU and memory of each local pane's process tree, shown as an overlay in
// the pane's top-right corner. One backend call for all panes every few
// seconds, only while the Terminals page is visible and the overlay is on.
import { get, writable } from 'svelte/store';
import { terminals } from '../stores/terminalStore.js';
import { currentPage, paneResourceOverlay } from '../stores/uiStore.js';

export const INTERVAL_MS = 3000;

/** terminal id → { cpu, rss, procs } */
export const paneResources = writable({});

let timer = null;

function app() {
  return window['go']?.['main']?.['App'];
}

export function startPaneResourceSampling() {
  if (timer) return;
  timer = setInterval(samplePaneResources, INTERVAL_MS);
  samplePaneResources();
}

export function stopPaneResourceSampling() {
  if (timer) clearInterval(timer);
  timer = null;
}

export async function samplePaneResources() {
  if (!get(paneResourceOverlay) || get(currentPage) !== 'terminals' || (typeof document !== 'undefined' && document.hidden)) {
    if (Object.keys(get(paneResources)).length) paneResources.set({});
    return;
  }
  const ids = get(terminals).filter((t) => t.type !== 'ssh' && !t.minimized).map((t) => t.id);
  if (!ids.length) {
    paneResources.set({});
    return;
  }
  const fn = app()?.GetPaneResourcesJSON;
  if (typeof fn !== 'function') return;
  try {
    const res = JSON.parse(await fn(JSON.stringify(ids)));
    paneResources.set(res?.panes || {});
  } catch {
    // keep the last reading; the next tick retries
  }
}

/** "12 % · 1.2 GB" */
export function formatResources(u) {
  if (!u) return '';
  const cpu = Math.round(u.cpu || 0);
  return `${cpu} % · ${formatBytes(u.rss || 0)}`;
}

export function formatBytes(bytes) {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(bytes >= 10 * 1024 ** 3 ? 0 : 1)} GB`;
  return `${Math.round(bytes / 1024 ** 2)} MB`;
}
