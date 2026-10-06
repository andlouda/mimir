import { beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { terminals } from '../stores/terminalStore.js';
import { applyTerminalBackground } from './backgroundActions.js';

vi.mock('@xterm/addon-webgl', () => ({ WebglAddon: class { onContextLoss() {} dispose() {} } }));
vi.mock('./terminalActions.js', () => ({ XTERM_THEME: { background: '#0c0e14', foreground: '#c9d1d9' } }));
vi.mock('./sessionActions.js', () => ({ scheduleSessionSave: () => {} }));

describe('pane background and xterm transparency', () => {
  let xterm;
  beforeEach(() => {
    xterm = { options: { allowTransparency: false, theme: {} }, loadAddon: vi.fn() };
    terminals.set([{ id: 1, name: 'A', terminal: xterm, webgl: null }]);
    global.window = { go: { main: { App: { UpdateTerminalBackground: async () => {}, SaveCurrentSession: async () => {} } } } };
  });

  test('transparency is on only while a picture is set', () => {
    const bg = applyTerminalBackground(1, { id: '0123456789abcdef.png' });
    expect(bg.id).toBe('0123456789abcdef.png');
    expect(xterm.options.allowTransparency).toBe(true);
    expect(xterm.options.theme.background).toMatch(/rgba/);
    applyTerminalBackground(1, null);
    expect(xterm.options.allowTransparency).toBe(false);
    expect(xterm.options.theme.background).toBe('#0c0e14');
    expect(get(terminals)[0].background).toBeNull();
  });
});
