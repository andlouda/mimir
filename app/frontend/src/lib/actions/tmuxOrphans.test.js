import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('../../../wailsjs/go/main/App', () => ({
  KillSSHTmuxSessionByName: vi.fn(),
  KillTmuxSession: vi.fn(),
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

const { orphanSessionCount } = await import('../stores/uiStore.js');
const {
  openRecoverableSessions,
  recoverableCount,
  recoverableSessions,
  refreshOrphanSessionCount,
  scheduleOrphanSessionRefresh,
} = await import('./tmuxSessionActions.js');

const row = (name, extra = {}) => ({ name, mimir: true, attached: 0, terminalId: 0, ...extra });
const group = {
  kind: 'ssh',
  profileId: 'p1',
  label: 'Pironman',
  sessions: [
    row('mimir-ssh-p1-a'),
    row('mimir-ssh-p1-b', { terminalId: 4 }), // shown in a pane
    row('mimir-ssh-p1-c', { attached: 1 }), // shown by another terminal
    row('main', { mimir: false }), // not created by Mimir
    row('mimir-ssh-p1-d'),
  ],
};

describe('sessions without a pane', () => {
  beforeEach(() => orphanSessionCount.set(0));

  test('only Mimir sessions that nothing shows count as recoverable', () => {
    expect(recoverableSessions(group).map((s) => s.name)).toEqual(['mimir-ssh-p1-a', 'mimir-ssh-p1-d']);
    expect(recoverableCount([group, { kind: 'local', sessions: [row('mimir-x')] }])).toBe(3);
    expect(recoverableCount([])).toBe(0);
    expect(recoverableSessions(undefined)).toEqual([]);
  });

  test('the sidebar count follows the hosts and drops to zero when asking fails', async () => {
    expect(await refreshOrphanSessionCount({ load: async () => [group] })).toBe(2);
    expect(get(orphanSessionCount)).toBe(2);
    expect(await refreshOrphanSessionCount({ load: async () => { throw new Error('no connection'); } })).toBe(0);
    expect(get(orphanSessionCount)).toBe(0);
  });

  test('open all brings back each recoverable session and nothing else', async () => {
    const open = vi.fn(async (_group, session) => (session.name.endsWith('a') ? 11 : null));
    expect(await openRecoverableSessions(group, { open })).toBe(1); // the second one failed to open
    expect(open.mock.calls.map(([, session]) => session.name)).toEqual(['mimir-ssh-p1-a', 'mimir-ssh-p1-d']);
  });
});

describe('scheduled refresh', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  test('repeated changes collapse into one query after the delay', () => {
    const refresh = vi.fn();
    scheduleOrphanSessionRefresh(5000, refresh);
    vi.advanceTimersByTime(3000);
    scheduleOrphanSessionRefresh(5000, refresh);
    vi.advanceTimersByTime(4999);
    expect(refresh).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(refresh).toHaveBeenCalledTimes(1);
  });
});

describe('stored arrangement', () => {
  test('panes that come back with a place are put together as one block', async () => {
    const { layoutTree } = await import('../stores/terminalStore.js');
    const { restoreArrangement, openRecoverableSessions: openAll } = await import('./tmuxSessionActions.js');
    const leaf = (terminalId) => ({ type: 'leaf', terminalId });
    const withPlace = {
      kind: 'ssh',
      profileId: 'p1',
      sessions: [
        row('mimir-ssh-p1-a', { meta: { v: 1, name: 'top', path: [{ d: 'v', r: 0.3, i: 0 }] } }),
        row('mimir-ssh-p1-b', { meta: { v: 1, name: 'bottom', path: [{ d: 'v', r: 0.3, i: 1 }] } }),
      ],
    };
    // A local pane is already there; each reopened pane is appended as a column first.
    layoutTree.set(leaf(1));
    let next = 10;
    const open = vi.fn(async () => {
      const id = next++;
      layoutTree.update((tree) => ({ type: 'split', direction: 'horizontal', ratio: 0.5, children: [tree, leaf(id)] }));
      return id;
    });
    expect(await openAll(withPlace, { open })).toBe(2);
    expect(get(layoutTree)).toEqual({
      type: 'split',
      direction: 'horizontal',
      ratio: 0.5,
      children: [leaf(1), { type: 'split', direction: 'vertical', ratio: 0.3, children: [leaf(10), leaf(11)] }],
    });

    // Contradicting places change nothing.
    const before = get(layoutTree);
    expect(restoreArrangement([{ id: 10, path: [] }, { id: 11, path: [] }])).toBe(false);
    expect(get(layoutTree)).toBe(before);
    expect(restoreArrangement([{ id: 10, path: [] }])).toBe(false);
  });
});
