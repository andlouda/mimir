import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { terminals, activeTerminalId, layoutTree } from '../stores/terminalStore.js';
import {
  CloseTerminal,
  CloseSSHTerminalFull,
  KillTmuxSession,
  StartRecording,
  StopRecording,
  ReconnectSSHTerminal,
  ConfirmFrontendReady,
  InitializeTerminal,
} from '../../../wailsjs/go/main/App';
import { safelyDisposeTerminal, safelyFitAndResizeTerminal } from '../terminals/xtermLifecycle.js';
import {
  handleResize,
  removeTerminal,
  toggleMinimize,
  toggleRecording,
  reconnectSSH,
  saveTerminalName,
} from './terminalActions.js';

vi.mock('@xterm/xterm', () => ({ Terminal: vi.fn() }));
vi.mock('@xterm/addon-fit', () => ({ FitAddon: vi.fn() }));
vi.mock('@xterm/addon-search', () => ({ SearchAddon: vi.fn() }));
vi.mock('@xterm/addon-clipboard', () => ({ ClipboardAddon: vi.fn() }));
vi.mock('../../../wailsjs/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
  ClipboardGetText: vi.fn(() => Promise.resolve('')),
  ClipboardSetText: vi.fn(() => Promise.resolve(true)),
}));

vi.mock('../../../wailsjs/go/main/App', () => ({
  StartTerminal: vi.fn(),
  StartSSHTerminal: vi.fn(),
  WriteToTerminal: vi.fn(),
  ResizeTerminal: vi.fn(),
  CloseTerminal: vi.fn(),
  CloseSSHTerminalFull: vi.fn(),
  InitializeTerminal: vi.fn(() => Promise.resolve()),
  ConfirmFrontendReady: vi.fn(() => Promise.resolve()),
  KillTmuxSession: vi.fn(() => ({ catch: () => {} })),
  StartRecording: vi.fn(() => Promise.resolve()),
  StopRecording: vi.fn(() => Promise.resolve()),
  RemoveTerminalState: vi.fn(),
  ReconnectSSHTerminal: vi.fn(() => Promise.resolve()),
}));

vi.mock('../terminals/xtermLifecycle.js', () => ({
  safelyWriteTerminal: vi.fn(),
  safelyFitAndResizeTerminal: vi.fn(),
  safelyAttachTerminal: vi.fn(() => true),
  safelyDisposeTerminal: vi.fn(),
}));

vi.mock('../terminals/reconnectLifecycle.js', () => ({
  markReconnectStarted: vi.fn((list) => list.map((t) => ({ ...t, reconnecting: true }))),
  markReconnectSucceeded: vi.fn((list) => list.map((t) => ({ ...t, reconnecting: false, disconnected: false }))),
  markReconnectFailed: vi.fn((list) => list.map((t) => ({ ...t, reconnecting: false }))),
}));

vi.mock('../transcript/transcriptApi.js', () => ({
  appendTerminalTranscript: vi.fn(),
  saveTranscriptMetadata: vi.fn(),
}));

vi.mock('./sessionActions.js', () => ({
  persistTerminalState: vi.fn(),
  scheduleSessionSave: vi.fn(),
}));

beforeEach(() => {
  vi.useFakeTimers();
  global.requestAnimationFrame = (cb) => cb();
  global.document = { getElementById: vi.fn(() => null) };
});

afterEach(() => {
  terminals.set([]);
  activeTerminalId.set(null);
  layoutTree.set(null);
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.clearAllMocks();
  delete global.requestAnimationFrame;
  delete global.document;
});

describe('handleResize', () => {
  test('fits visible terminals only', () => {
    terminals.set([
      { id: 1, minimized: false },
      { id: 2, minimized: true },
    ]);

    handleResize();

    expect(safelyFitAndResizeTerminal).toHaveBeenCalledTimes(1);
    expect(safelyFitAndResizeTerminal.mock.calls[0][0].id).toBe(1);
  });
});

describe('removeTerminal', () => {
  test('kills owned tmux session and closes the backend terminal', () => {
    terminals.set([{ id: 5, type: 'bash', tmuxSessionName: 'mimir-x', tmuxOwned: true }]);

    removeTerminal(5);

    expect(KillTmuxSession).toHaveBeenCalledWith('mimir-x');
    expect(CloseTerminal).toHaveBeenCalledWith(5);
  });

  test('disconnected SSH terminals are removed immediately', () => {
    terminals.set([{ id: 9, type: 'ssh', disconnected: true, cleanupHandlers: [] }]);
    layoutTree.set({ type: 'leaf', terminalId: 9 });

    removeTerminal(9);

    expect(CloseSSHTerminalFull).toHaveBeenCalledWith(9);
    expect(safelyDisposeTerminal).toHaveBeenCalled();
    expect(get(terminals)).toHaveLength(0);
  });
});

describe('toggleRecording', () => {
  test('starts recording for an idle terminal', async () => {
    terminals.set([{ id: 1, name: 'Shell', recording: false }]);

    await toggleRecording(1);

    expect(StartRecording).toHaveBeenCalledWith(1, 'Shell');
    expect(get(terminals)[0].recording).toBe(true);
  });

  test('stops recording for an active terminal', async () => {
    terminals.set([{ id: 1, name: 'Shell', recording: true }]);

    await toggleRecording(1);

    expect(StopRecording).toHaveBeenCalledWith(1);
    expect(get(terminals)[0].recording).toBe(false);
  });
});

describe('toggleMinimize', () => {
  test('minimizing removes the leaf from the layout tree', async () => {
    terminals.set([{ id: 1, minimized: false, cleanupHandlers: [] }]);
    layoutTree.set({ type: 'leaf', terminalId: 1 });

    await toggleMinimize(1);

    expect(get(terminals)[0].minimized).toBe(true);
    expect(get(layoutTree)).toBeNull();
  });

  test('restoring re-inserts the leaf into the layout tree', async () => {
    terminals.set([{ id: 1, minimized: true, cleanupHandlers: [] }]);
    layoutTree.set(null);

    await toggleMinimize(1);

    expect(get(terminals)[0].minimized).toBe(false);
    expect(get(layoutTree)).toEqual({ type: 'leaf', terminalId: 1 });
  });
});

describe('reconnectSSH', () => {
  test('marks the terminal reconnecting then succeeded', async () => {
    terminals.set([{ id: 4, disconnected: true, reconnecting: false }]);

    await reconnectSSH(4);

    expect(ReconnectSSHTerminal).toHaveBeenCalledWith(4);
    expect(ConfirmFrontendReady).toHaveBeenCalledWith(4);
    expect(InitializeTerminal).toHaveBeenCalledWith(4);
    expect(get(terminals)[0].disconnected).toBe(false);
  });
});

describe('saveTerminalName', () => {
  test('updates the terminal name and exits edit mode', () => {
    terminals.set([{ id: 2, name: 'Old', editingName: true, type: 'bash', resumeId: 'r', sshProfileId: '' }]);

    saveTerminalName(2, { target: { value: 'New' } });

    const term = get(terminals)[0];
    expect(term.name).toBe('New');
    expect(term.editingName).toBe(false);
  });
});
