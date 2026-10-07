// Live counters for the Settings diagnostics card. Everything here is
// cheap by construction: callers only bump integers; the sampler (started
// while the card is open) folds them into a per-second snapshot. The
// main-thread estimate measures how late a 100 ms timer fires: time the
// thread was busy with JS, layout or painting shows up as drift.
import { writable } from 'svelte/store';

const TICK_MS = 100;

const counters = { events: 0, bytes: 0, writes: 0 };

export const perfStats = writable(null);

export function countOutput(bytes) {
  counters.events += 1;
  counters.bytes += bytes;
}

export function countWrite() {
  counters.writes += 1;
}

let timer = null;
let sampler = null;

export function startDiagnostics({ getPanes = () => document.querySelectorAll('.xterm').length, getAnimations = () => (document.getAnimations?.() || []).length } = {}) {
  if (timer) return;
  let last = performance.now();
  let busy = 0;
  let windowStart = last;
  let e0 = counters.events, b0 = counters.bytes, w0 = counters.writes;
  timer = setInterval(() => {
    const now = performance.now();
    busy += Math.max(0, now - last - TICK_MS);
    last = now;
  }, TICK_MS);
  sampler = setInterval(() => {
    const now = performance.now();
    const span = Math.max(1, now - windowStart);
    perfStats.set({
      mainThreadBusy: Math.min(100, Math.round((busy / span) * 100)),
      eventsPerSec: Math.round(((counters.events - e0) * 1000) / span),
      kbPerSec: Math.round(((counters.bytes - b0) * 1000) / span / 1024),
      writesPerSec: Math.round(((counters.writes - w0) * 1000) / span),
      panes: getPanes(),
      animations: getAnimations(),
    });
    busy = 0; windowStart = now;
    e0 = counters.events; b0 = counters.bytes; w0 = counters.writes;
  }, 1000);
}

export function stopDiagnostics() {
  if (timer) clearInterval(timer);
  if (sampler) clearInterval(sampler);
  timer = sampler = null;
  perfStats.set(null);
}
