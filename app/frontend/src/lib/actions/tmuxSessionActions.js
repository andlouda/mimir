// The tmux session overview: every session on the connected SSH hosts and
// on Mimir's local tmux server, with the program in it and whether a pane
// shows it. Sessions that lost their pane (dropped connection, crash, an
// older Mimir that mixed up names) stay findable here and can be opened in
// a pane again or ended.
import { get } from 'svelte/store';
import { tick } from 'svelte';
import { KillSSHTmuxSessionByName, KillTmuxSession, ListTmuxSessionsJSON, StartSSHTerminalSession, StartTerminalWithOptions } from '../backend.js';
import { activeTerminalId, layoutTree, terminals } from '../stores/terminalStore.js';
import { currentPage, errorMessage, showTmuxSessions } from '../stores/uiStore.js';
import { appendLeaf } from '../terminals/layoutTree.js';
import { focusTerminal } from './keyboardShortcuts.js';
import { persistTerminalState } from './sessionActions.js';
import { createTerminalInstance, reinitializeTerminals, terminalToForeground } from './terminalActions.js';

export function openTmuxSessions() {
  showTmuxSessions.set(true);
}

export function closeTmuxSessions() {
  showTmuxSessions.set(false);
}

/** Groups from the backend: [{ kind, label, profileId, terminalId, terminalType, error, sessions: [...] }]. */
export async function loadTmuxSessions({ list = ListTmuxSessionsJSON } = {}) {
  const groups = JSON.parse((await list()) || '[]');
  return Array.isArray(groups) ? groups.map((g) => ({ ...g, sessions: Array.isArray(g.sessions) ? g.sessions : [] })) : [];
}

/**
 * Where a session is shown:
 *   'pane'     a Mimir pane of this window shows it
 *   'attached' some other tmux client shows it (another terminal or machine)
 *   'orphan'   nothing shows it; it only keeps running on the host
 */
export function sessionStatus(row) {
  if (row?.terminalId) return 'pane';
  if ((row?.attached || 0) > 0) return 'attached';
  return 'orphan';
}

/** Only sessions Mimir created may be opened or ended; one a pane shows is ended by closing the pane. */
export function canOpenSession(row) {
  return Boolean(row?.mimir) && !row?.terminalId;
}

export function canKillSession(row) {
  return Boolean(row?.mimir) && !row?.terminalId;
}

/** Number of sessions nothing is attached to, across all groups. */
export function orphanCount(groups) {
  return (groups || []).reduce((n, g) => n + g.sessions.filter((s) => sessionStatus(s) === 'orphan').length, 0);
}

/** Brings the pane that shows a session to the front. */
export async function showSessionPane(row) {
  const term = get(terminals).find((t) => t.id === row?.terminalId);
  if (!term) return false;
  closeTmuxSessions();
  if (get(currentPage) !== 'terminals') {
    currentPage.set('terminals');
    await tick();
  }
  if (term.minimized) await terminalToForeground(term.id);
  focusTerminal(term.id);
  return true;
}

/**
 * Opens a session in a new pane. tmux attaches with "new-session -A -D":
 * another client that shows the session is detached.
 */
export async function openTmuxSession(group, row, deps = {}) {
  const {
    startSSH = StartSSHTerminalSession,
    startLocal = StartTerminalWithOptions,
    createInstance = createTerminalInstance,
    persist = persistTerminalState,
    reinitialize = reinitializeTerminals,
  } = deps;
  if (!canOpenSession(row)) return null;
  try {
    const ssh = group.kind === 'ssh';
    const type = ssh ? 'ssh' : group.terminalType;
    const id = ssh ? await startSSH(group.profileId, row.name) : await startLocal(type, row.name);
    if (!id) {
      errorMessage.set('Failed to open the tmux session. The backend returned an invalid ID.');
      return null;
    }
    closeTmuxSessions();
    if (get(currentPage) !== 'terminals') {
      currentPage.set('terminals');
      await tick();
    }
    layoutTree.set(appendLeaf(get(layoutTree), { type: 'leaf', terminalId: id }));
    const name = ssh ? `SSH: ${group.label}` : `${String(type).toUpperCase()} ${get(terminals).length + 1}`;
    const term = await createInstance(id, type, name, false, ssh ? group.profileId : '', false, row.name, '', 'fresh');
    if (!term) return null; // closed while it was still starting
    activeTerminalId.set(id);
    persist(ssh ? { ...term, type: 'ssh', minimized: false, sshProfileId: group.profileId } : { ...term, minimized: false });
    await reinitialize();
    return id;
  } catch (error) {
    errorMessage.set(`Failed to open tmux session ${row.name}: ${error?.message || error}`);
    return null;
  }
}

/** Ends a session and everything running in it. Returns true when the request went through. */
export async function killTmuxSession(group, row, { killSSH = KillSSHTmuxSessionByName, killLocal = KillTmuxSession } = {}) {
  if (!canKillSession(row)) return false;
  try {
    if (group.kind === 'ssh') await killSSH(group.terminalId, row.name);
    else await killLocal(row.name);
    return true;
  } catch (error) {
    errorMessage.set(`Failed to end tmux session ${row.name}: ${error?.message || error}`);
    return false;
  }
}

/** "3 h", "2 d", "5 min": how long ago a unix timestamp (seconds) was. */
export function sessionAge(created, now = Date.now()) {
  if (!created) return '';
  const minutes = Math.max(0, Math.floor((now / 1000 - created) / 60));
  if (minutes < 60) return `${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours} h`;
  return `${Math.floor(hours / 24)} d`;
}
