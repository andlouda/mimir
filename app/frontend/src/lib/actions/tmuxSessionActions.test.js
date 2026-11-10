import { beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('../../../wailsjs/go/main/App', () => ({
  KillSSHTmuxSessionByName: vi.fn(() => Promise.resolve()),
  KillTmuxSession: vi.fn(() => Promise.resolve()),
  ListTmuxSessionsJSON: vi.fn(),
  StartSSHTerminalSession: vi.fn(),
  StartTerminalWithOptions: vi.fn(),
}));
vi.mock('./terminalActions.js', () => ({
  createTerminalInstance: vi.fn(),
  reinitializeTerminals: vi.fn(() => Promise.resolve()),
  terminalToForeground: vi.fn(() => Promise.resolve()),
}));
vi.mock('./sessionActions.js', () => ({ persistTerminalState: vi.fn() }));
vi.mock('./keyboardShortcuts.js', () => ({ focusTerminal: vi.fn() }));

const { KillSSHTmuxSessionByName, KillTmuxSession } = await import('../../../wailsjs/go/main/App');
const { terminalToForeground } = await import('./terminalActions.js');
const { focusTerminal } = await import('./keyboardShortcuts.js');
const {
  canKillSession,
  canOpenSession,
  killTmuxSession,
  loadTmuxSessions,
  openTmuxSession,
  orphanCount,
  sessionAge,
  sessionStatus,
  showSessionPane,
} = await import('./tmuxSessionActions.js');
const { activeTerminalId, layoutTree, terminals } = await import('../stores/terminalStore.js');
const { currentPage, errorMessage, showTmuxSessions } = await import('../stores/uiStore.js');

const sshGroup = { kind: 'ssh', label: 'Pironman', profileId: 'p1', terminalId: 3, terminalType: 'ssh', sessions: [] };
const localGroup = { kind: 'local', label: 'local', terminalType: 'wsl', sessions: [] };
const orphan = { name: 'mimir-ssh-p1-2', attached: 0, mimir: true, terminalId: 0, command: 'claude', path: '/srv/app' };

beforeEach(() => {
  vi.clearAllMocks();
  terminals.set([]);
  layoutTree.set(null);
  activeTerminalId.set(null);
  currentPage.set('terminals');
  errorMessage.set('');
  showTmuxSessions.set(true);
});

describe('session rules', () => {
  test('status tells panes, other clients and orphans apart', () => {
    expect(sessionStatus({ terminalId: 4, attached: 1 })).toBe('pane');
    expect(sessionStatus({ terminalId: 0, attached: 1 })).toBe('attached');
    expect(sessionStatus({ terminalId: 0, attached: 0 })).toBe('orphan');
  });

  test('only Mimir sessions without a pane can be opened or ended', () => {
    expect(canOpenSession(orphan)).toBe(true);
    expect(canKillSession(orphan)).toBe(true);
    expect(canOpenSession({ ...orphan, mimir: false })).toBe(false);
    expect(canKillSession({ ...orphan, mimir: false })).toBe(false);
    expect(canOpenSession({ ...orphan, terminalId: 4 })).toBe(false);
    expect(canKillSession({ ...orphan, terminalId: 4 })).toBe(false);
  });

  test('orphans are counted across groups', () => {
    const groups = [
      { sessions: [orphan, { ...orphan, attached: 1 }] },
      { sessions: [{ ...orphan, terminalId: 2 }, orphan] },
    ];
    expect(orphanCount(groups)).toBe(2);
    expect(orphanCount([])).toBe(0);
  });

  test('age reads as minutes, hours, days', () => {
    const now = 1_000_000_000_000;
    const ago = (seconds) => now / 1000 - seconds;
    expect(sessionAge(ago(5 * 60), now)).toBe('5 min');
    expect(sessionAge(ago(3 * 3600), now)).toBe('3 h');
    expect(sessionAge(ago(3 * 86400), now)).toBe('3 d');
    expect(sessionAge(0, now)).toBe('');
  });
});

describe('loadTmuxSessions', () => {
  test('parses groups and tolerates missing session lists', async () => {
    const list = vi.fn(() => Promise.resolve(JSON.stringify([{ kind: 'ssh', label: 'a', sessions: [orphan] }, { kind: 'local', label: 'local', sessions: null }])));
    const groups = await loadTmuxSessions({ list });
    expect(groups).toHaveLength(2);
    expect(groups[0].sessions[0].name).toBe('mimir-ssh-p1-2');
    expect(groups[1].sessions).toEqual([]);
    expect(await loadTmuxSessions({ list: () => Promise.resolve('') })).toEqual([]);
  });
});

describe('openTmuxSession', () => {
  const deps = () => ({
    startSSH: vi.fn(() => Promise.resolve(11)),
    startLocal: vi.fn(() => Promise.resolve(12)),
    createInstance: vi.fn((id, type) => Promise.resolve({ id, type })),
    persist: vi.fn(),
    reinitialize: vi.fn(() => Promise.resolve()),
  });

  test('an orphaned SSH session opens in a new pane attached to that session', async () => {
    const d = deps();
    const id = await openTmuxSession(sshGroup, orphan, d);
    expect(id).toBe(11);
    expect(d.startSSH).toHaveBeenCalledWith('p1', 'mimir-ssh-p1-2');
    expect(d.createInstance).toHaveBeenCalledWith(11, 'ssh', 'SSH: Pironman', false, 'p1', false, 'mimir-ssh-p1-2', '', 'fresh');
    expect(d.persist).toHaveBeenCalledWith(expect.objectContaining({ type: 'ssh', sshProfileId: 'p1', minimized: false }));
    expect(get(activeTerminalId)).toBe(11);
    expect(get(layoutTree)).toEqual({ type: 'leaf', terminalId: 11 });
    expect(get(showTmuxSessions)).toBe(false);
  });

  test('a local session opens through the local tmux pane type', async () => {
    const d = deps();
    const id = await openTmuxSession(localGroup, { ...orphan, name: 'mimir-abc' }, d);
    expect(id).toBe(12);
    expect(d.startLocal).toHaveBeenCalledWith('wsl', 'mimir-abc');
    expect(d.startSSH).not.toHaveBeenCalled();
    expect(d.createInstance).toHaveBeenCalledWith(12, 'wsl', 'WSL 1', false, '', false, 'mimir-abc', '', 'fresh');
  });

  test('foreign sessions and sessions in a pane are not opened', async () => {
    const d = deps();
    expect(await openTmuxSession(sshGroup, { ...orphan, mimir: false }, d)).toBeNull();
    expect(await openTmuxSession(sshGroup, { ...orphan, terminalId: 4 }, d)).toBeNull();
    expect(d.startSSH).not.toHaveBeenCalled();
  });

  test('a failing start is reported and leaves the overview open', async () => {
    const d = deps();
    d.startSSH = vi.fn(() => Promise.reject(new Error('connection refused')));
    expect(await openTmuxSession(sshGroup, orphan, d)).toBeNull();
    expect(get(errorMessage)).toContain('connection refused');
    expect(get(showTmuxSessions)).toBe(true);
  });
});

describe('killTmuxSession', () => {
  test('SSH sessions end over the group pane, local ones on the local server', async () => {
    expect(await killTmuxSession(sshGroup, orphan)).toBe(true);
    expect(KillSSHTmuxSessionByName).toHaveBeenCalledWith(3, 'mimir-ssh-p1-2');
    expect(await killTmuxSession(localGroup, { ...orphan, name: 'mimir-abc' })).toBe(true);
    expect(KillTmuxSession).toHaveBeenCalledWith('mimir-abc');
  });

  test('foreign sessions and sessions in a pane are never ended', async () => {
    expect(await killTmuxSession(sshGroup, { ...orphan, mimir: false })).toBe(false);
    expect(await killTmuxSession(sshGroup, { ...orphan, terminalId: 4 })).toBe(false);
    expect(KillSSHTmuxSessionByName).not.toHaveBeenCalled();
  });

  test('a backend refusal is reported', async () => {
    KillSSHTmuxSessionByName.mockRejectedValueOnce(new Error('open in a pane'));
    expect(await killTmuxSession(sshGroup, orphan)).toBe(false);
    expect(get(errorMessage)).toContain('open in a pane');
  });
});

describe('showSessionPane', () => {
  test('brings a minimized pane back and focuses it', async () => {
    terminals.set([{ id: 4, minimized: true }]);
    expect(await showSessionPane({ terminalId: 4 })).toBe(true);
    expect(terminalToForeground).toHaveBeenCalledWith(4);
    expect(focusTerminal).toHaveBeenCalledWith(4);
    expect(get(showTmuxSessions)).toBe(false);
    expect(await showSessionPane({ terminalId: 99 })).toBe(false);
  });
});
