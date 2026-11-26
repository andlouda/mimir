// Keeps each SSH pane's name and place with its tmux session on the host,
// for profiles that opted in ("layoutOnHost"). See terminals/hostLayout.js
// for the format and app/app_pane_meta.go for what the backend accepts.
//
// Only panes of opted-in profiles are ever sent, only when their placement
// changed, and a little after the last change so that dragging a divider
// does not produce a write per pixel.
import { get } from 'svelte/store';
import { optional } from '../backend.js';
import { sshProfiles } from '../stores/sshStore.js';
import { layoutTree, terminals } from '../stores/terminalStore.js';
import { leafPaths, paneMetaFor } from '../terminals/hostLayout.js';

const lastSent = new Map(); // terminal id → JSON last stored on the host

/** Panes whose placement belongs on their host: [{ id, json }]. */
export function hostLayoutPayloads(terminalList, tree, profiles) {
  const optedIn = new Set((profiles || []).filter((profile) => profile?.layoutOnHost).map((profile) => profile.id));
  if (optedIn.size === 0) return [];
  const paths = leafPaths(tree);
  return (terminalList || [])
    .filter((term) => term.type === 'ssh' && term.tmuxActive && term.tmuxSessionName && !term.disconnected && optedIn.has(term.sshProfileId))
    .map((term) => ({ id: term.id, json: JSON.stringify(paneMetaFor(term, paths.get(term.id) || [])) }));
}

/** Sends what changed. Returns how many panes were written. */
export async function syncHostLayout({ save = optional('SaveSSHPaneMeta') } = {}) {
  if (typeof save !== 'function') return 0;
  let written = 0;
  for (const { id, json } of hostLayoutPayloads(get(terminals), get(layoutTree), get(sshProfiles))) {
    if (lastSent.get(id) === json) continue;
    try {
      await save(id, json);
      lastSent.set(id, json);
      written += 1;
    } catch (error) {
      // The pane may still be logging in; the next change tries again.
      console.warn('Failed to store pane placement on the host:', error);
    }
  }
  return written;
}

let timer = null;

export function scheduleHostLayoutSync(delayMs = 2000, sync = syncHostLayout) {
  if (timer) clearTimeout(timer);
  timer = setTimeout(() => {
    timer = null;
    sync();
  }, delayMs);
}

/** For tests. */
export function resetHostLayoutSync() {
  lastSent.clear();
  if (timer) clearTimeout(timer);
  timer = null;
}
