import { beforeEach, describe, expect, test, vi } from 'vitest';
import { sshProfiles } from '../stores/sshStore.js';
import { layoutTree, terminals } from '../stores/terminalStore.js';
import { hostLayoutPayloads, resetHostLayoutSync, syncHostLayout } from './hostLayoutSync.js';

const leaf = (terminalId) => ({ type: 'leaf', terminalId });
const tree = { type: 'split', direction: 'horizontal', ratio: 0.5, children: [leaf(1), leaf(2)] };
const ssh = (id, extra = {}) => ({ id, type: 'ssh', name: `SSH ${id}`, tmuxActive: true, tmuxSessionName: `mimir-ssh-p1-${id}`, sshProfileId: 'p1', ...extra });

describe('placement on the host', () => {
  beforeEach(() => {
    resetHostLayoutSync();
    terminals.set([ssh(1), ssh(2, { sshProfileId: 'p2' }), { id: 3, type: 'wsl', name: 'WSL', tmuxActive: true, tmuxSessionName: 'mimir-x' }]);
    layoutTree.set(tree);
    sshProfiles.set([{ id: 'p1', layoutOnHost: true }, { id: 'p2' }]);
  });

  test('only panes of profiles that opted in are sent', () => {
    const payloads = hostLayoutPayloads([ssh(1), ssh(2, { sshProfileId: 'p2' })], tree, [{ id: 'p1', layoutOnHost: true }, { id: 'p2' }]);
    expect(payloads).toHaveLength(1);
    expect(payloads[0].id).toBe(1);
    expect(JSON.parse(payloads[0].json)).toEqual({ v: 1, name: 'SSH 1', path: [{ d: 'h', r: 0.5, i: 0 }] });
    expect(hostLayoutPayloads([ssh(1)], tree, [{ id: 'p1' }])).toEqual([]);
  });

  test('disconnected panes and panes without tmux are skipped', () => {
    const profiles = [{ id: 'p1', layoutOnHost: true }];
    expect(hostLayoutPayloads([ssh(1, { disconnected: true })], tree, profiles)).toEqual([]);
    expect(hostLayoutPayloads([ssh(1, { tmuxActive: false })], tree, profiles)).toEqual([]);
  });

  test('a pane is written once and again only when its placement changes', async () => {
    const save = vi.fn(async () => {});
    expect(await syncHostLayout({ save })).toBe(1);
    expect(await syncHostLayout({ save })).toBe(0);
    layoutTree.set({ ...tree, ratio: 0.7 });
    expect(await syncHostLayout({ save })).toBe(1);
    expect(save).toHaveBeenCalledTimes(2);
    expect(save.mock.calls[1][0]).toBe(1);
  });

  test('a failed write is tried again next time', async () => {
    const save = vi.fn().mockRejectedValueOnce(new Error('no connection')).mockResolvedValue(undefined);
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    expect(await syncHostLayout({ save })).toBe(0);
    expect(await syncHostLayout({ save })).toBe(1);
  });

  test('without the backend method nothing happens', async () => {
    expect(await syncHostLayout({ save: undefined })).toBe(0);
  });
});
