// Regression test for terminals that are created or restored while
// minimized: their keyboard path and DOM handlers must be wired even though
// no DOM element exists at creation, and the DOM handlers must be installed
// once the terminal is shown.
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { terminals, activeTerminalId, layoutTree } from '../stores/terminalStore.js';
import { WriteToTerminal } from '../../../wailsjs/go/main/App';
import { observeTerminalResize } from '../terminals/xtermLifecycle.js';
import { createTerminalInstance, terminalToForeground, wireTerminalDom } from './terminalActions.js';

const listeners = new Map();
const fakeXtermElement = {
  addEventListener: vi.fn((type, fn) => listeners.set(type, fn)),
  removeEventListener: vi.fn((type) => listeners.delete(type)),
};

let lastTerminal = null;

vi.mock('@xterm/xterm', () => ({
  Terminal: vi.fn(function FakeTerminal() {
    this.element = null;
    this.rows = 24;
    this.cols = 80;
    this.loadAddon = vi.fn();
    this.onData = vi.fn((fn) => { this._onData = fn; return { dispose: vi.fn() }; });
    this.onTitleChange = vi.fn(() => ({ dispose: vi.fn() }));
    this.open = vi.fn(() => { this.element = fakeXtermElement; });
    this.focus = vi.fn();
    this.write = vi.fn();
    this.paste = vi.fn();
    this.dispose = vi.fn();
    lastTerminal = this;
  }),
}));
vi.mock('@xterm/addon-fit', () => ({ FitAddon: vi.fn(function () { this.fit = vi.fn(); }) }));
vi.mock('@xterm/addon-search', () => ({ SearchAddon: vi.fn(function () {}) }));
vi.mock('@xterm/addon-clipboard', () => ({ ClipboardAddon: vi.fn(function () {}) }));
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: vi.fn(function () {}) }));
vi.mock('../../../wailsjs/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
  BrowserOpenURL: vi.fn(),
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
vi.mock('../terminals/xtermLifecycle.js', async () => {
  const actual = await vi.importActual('../terminals/xtermLifecycle.js');
  return { ...actual, observeTerminalResize: vi.fn(() => () => {}) };
});
vi.mock('../transcript/transcriptApi.js', () => ({
  appendTerminalTranscript: vi.fn(),
  saveTranscriptMetadata: vi.fn(),
}));
vi.mock('./sessionActions.js', () => ({
  persistTerminalState: vi.fn(),
  scheduleSessionSave: vi.fn(),
}));
vi.mock('./agentActions.js', () => ({
  handleTerminalPrompt: vi.fn(),
  handleTerminalTitle: vi.fn(),
  noteTerminalOutput: vi.fn(),
  startAgentWatch: vi.fn(),
  stopAgentWatch: vi.fn(),
}));

beforeEach(() => {
  global.requestAnimationFrame = (cb) => cb();
  global.document = { getElementById: vi.fn(() => null) };
  globalThis.window = { go: { main: { App: { GetTerminalTmuxStatus: vi.fn(() => Promise.resolve({ active: true, sessionName: 'mimir-local-7', status: 'active' })) } } } };
  listeners.clear();
  lastTerminal = null;
});

afterEach(() => {
  terminals.set([]);
  activeTerminalId.set(null);
  layoutTree.set(null);
  vi.clearAllMocks();
  delete global.requestAnimationFrame;
  delete global.document;
  delete globalThis.window;
});

describe('minimized terminal restore', () => {
  test('input works before the terminal was ever shown, DOM handlers arrive on first show', async () => {
    // Restored minimized: no DOM element exists.
    await createTerminalInstance(7, 'bash', 'BASH 1', true, '', true, 'mimir-local-7', 'resume-7', 'rehydrated');
    const term = get(terminals).find((t) => t.id === 7);
    expect(term.minimized).toBe(true);
    expect(lastTerminal.onData).toHaveBeenCalledTimes(1);
    expect(lastTerminal.onTitleChange).toHaveBeenCalledTimes(1);
    expect(lastTerminal.open).not.toHaveBeenCalled();
    expect(observeTerminalResize).not.toHaveBeenCalled();

    // Keystrokes reach the backend even though nothing is rendered yet.
    lastTerminal._onData('ls\r');
    expect(WriteToTerminal).toHaveBeenCalledWith(7, 'ls\r');

    // Now the user un-minimizes: the container exists, xterm is opened and
    // the DOM-dependent handlers are installed exactly once.
    const container = { replaceChildren: vi.fn() };
    global.document.getElementById = vi.fn((id) => (id === 'terminal-7' ? container : null));
    await terminalToForeground(7);
    expect(lastTerminal.open).toHaveBeenCalledWith(container);
    expect(observeTerminalResize).toHaveBeenCalledTimes(1);
    expect(fakeXtermElement.addEventListener).toHaveBeenCalledWith('paste', expect.any(Function));

    // Paste routes through xterm's bracketed paste.
    const preventDefault = vi.fn();
    listeners.get('paste')({ clipboardData: { getData: () => 'echo hi\n' }, preventDefault });
    expect(lastTerminal.paste).toHaveBeenCalledWith('echo hi\n');
    expect(preventDefault).toHaveBeenCalled();

    // Re-attaching (layout change) must not double-wire.
    const again = get(terminals).find((t) => t.id === 7);
    wireTerminalDom(again);
    expect(observeTerminalResize).toHaveBeenCalledTimes(1);
  });

  test('visible terminals are wired at creation as before', async () => {
    const container = { replaceChildren: vi.fn() };
    global.document.getElementById = vi.fn(() => container);
    await createTerminalInstance(8, 'bash', 'BASH 2', false, '', false, '', 'resume-8', 'fresh');
    expect(lastTerminal.open).toHaveBeenCalledWith(container);
    expect(lastTerminal.onData).toHaveBeenCalledTimes(1);
    expect(observeTerminalResize).toHaveBeenCalledTimes(1);
  });
});
