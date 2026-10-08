// CPU and memory of each local pane's process tree, shown as an overlay in
// the pane's top-right corner. One backend call for all panes every few
// seconds, only while the Terminals page is visible and the overlay is on.
import { get, writable } from 'svelte/store';
import { terminals } from '../stores/terminalStore.js';
import { currentPage, paneResourceOverlay } from '../stores/uiStore.js';

export const INTERVAL_MS = 3000;

/** terminal id → { cpu, rss, procs } */
export const paneResources = writable({});
/** Logical cores of the machine, as reported by the backend (0 = unknown). */
export const paneResourceCores = writable(0);

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
    if (res?.cores > 0) paneResourceCores.set(res.cores);
  } catch {
    // keep the last reading; the next tick retries
  }
}

/**
 * "3.4 CPU · 1.2 GB". The CPU share arrives per core (100 = one core busy);
 * scale 'cores' shows CPUs in use, 'machine' divides the percent by the
 * core count, 'core' keeps the per-core percent. Without a known core count
 * 'machine' falls back to per core.
 */
export function formatResources(u, scale = 'cores', cores = 0) {
  if (!u) return '';
  const perCore = u.cpu || 0;
  let cpu;
  if (scale === 'cores') cpu = `${(perCore / 100).toFixed(perCore >= 1000 ? 0 : 1)} CPU`;
  else if (scale === 'machine' && cores > 0) cpu = `${formatPercent(perCore / cores)} %`;
  else cpu = `${formatPercent(perCore)} %`;
  return `${cpu} · ${formatBytes(u.rss || 0)}`;
}

// Small shares keep one decimal so a pane at a quarter core does not read
// as 0 % of a many-core machine.
function formatPercent(v) {
  return v > 0 && v < 10 ? v.toFixed(1) : String(Math.round(v));
}

export function formatBytes(bytes) {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(bytes >= 10 * 1024 ** 3 ? 0 : 1)} GB`;
  return `${Math.round(bytes / 1024 ** 2)} MB`;
}
