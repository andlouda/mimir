import { get } from 'svelte/store';
import { tick } from 'svelte';
import { terminals, activeTerminalId, layoutTree } from '../stores/terminalStore.js';
import { currentPage, errorMessage, promptMode, terminalFontSize, terminalRenderer } from '../stores/uiStore.js';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { WebglAddon } from '@xterm/addon-webgl';
import { sshProfiles } from '../stores/sshStore.js';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { SearchAddon } from '@xterm/addon-search';
import { ClipboardAddon } from '@xterm/addon-clipboard';
import { createWriteOnlyClipboardProvider } from '../terminals/osc52Clipboard.js';
import { clearHoveredLink, createLinkProvider } from '../terminals/terminalLinks.js';
import { isGlobalShortcut, closeTopOverlay, focusTerminal } from './keyboardShortcuts.js';
import { ClipboardSetText, EventsOn } from '../../../wailsjs/runtime';
import { WriteToTerminal, ResizeTerminal, CloseTerminal, InitializeTerminal, ConfirmFrontendReady, StartTerminal, StartSSHTerminal, CloseSSHTerminalFull, KillTmuxSession, StartRecording, StopRecording, RemoveTerminalState, ReconnectSSHTerminal } from '../../../wailsjs/go/main/App';
import { replaceLeaf, removeLeafFromTree, collectLeafIds, appendLeaf } from '../terminals/layoutTree.js';
import { generateTmuxSessionName } from '../terminals/tmuxLifecycle.js';
import { containsControlChars, generateResumeId, shellQuotePath } from '../util.js';
import { handleTerminalPrompt, handleTerminalTitle, noteTerminalOutput, startAgentWatch, stopAgentWatch } from './agentActions.js';
import { safelyWriteTerminal, safelyFitAndResizeTerminal, safelyAttachTerminal, safelyDisposeTerminal, observeTerminalResize, rebindTerminalResize, forgetTerminalResize, enableWebgl, disableWebgl } from '../terminals/xtermLifecycle.js';
import { markReconnectStarted, markReconnectSucceeded, markReconnectFailed } from '../terminals/reconnectLifecycle.js';
import { appendTerminalTranscript, saveTranscriptMetadata } from '../transcript/transcriptApi.js';
import { persistTerminalState, scheduleSessionSave } from './sessionActions.js';

const tmuxCapableTerminalTypes = new Set(['bash', 'zsh', 'wsl']);

const XTERM_THEME = {
  // xterm 6 draws the overview ruler's border in white unless themed.
  overviewRulerBorder: '#1c2033',
  background: '#0c0e14',
  // xterm 6's own scrollbar slider; without these it falls back to the
  // foreground colour at 20% (a light grey bar).
  scrollbarSliderBackground: 'rgba(99, 179, 237, 0.22)',
  scrollbarSliderHoverBackground: 'rgba(99, 179, 237, 0.4)',
  scrollbarSliderActiveBackground: 'rgba(99, 179, 237, 0.55)',
  foreground: '#c9d1d9',
  cursor: '#63b3ed',
  cursorAccent: '#0c0e14',
  selectionBackground: 'rgba(99, 179, 237, 0.25)',
  selectionForeground: '#ffffff',
  black: '#1a1e2e',
  red: '#f47067',
  green: '#7ee787',
  yellow: '#e3b341',
  blue: '#63b3ed',
  magenta: '#d2a8ff',
  cyan: '#76e4f7',
  white: '#c9d1d9',
  brightBlack: '#545d68',
  brightRed: '#ff7b72',
  brightGreen: '#7ee787',
  brightYellow: '#f0c74f',
  brightBlue: '#79c0ff',
  brightMagenta: '#d6b4fc',
  brightCyan: '#9aedfe',
  brightWhite: '#f0f3f6'
};

async function startTerminalBackend(type, tmuxSessionName = '') {
  const startWithOptions = window['go']?.['main']?.['App']?.['StartTerminalWithOptions'];
  if (typeof startWithOptions === 'function') {
    return startWithOptions(type, tmuxSessionName);
  }
  return StartTerminal(type);
}

async function readTmuxStatus(id) {
  const getStatus = window['go']?.['main']?.['App']?.['GetTerminalTmuxStatus'];
  if (typeof getStatus === 'function') {
    return getStatus(id);
  }
  const getSSHStatus = window['go']?.['main']?.['App']?.['GetSSHTerminalTmuxStatus'];
  if (typeof getSSHStatus === 'function') {
    return getSSHStatus(id);
  }
  return { active: false, sessionName: '' };
}

export function cleanupTerminalResources(term, { dispose = true } = {}) {
  if (!term) return;
  for (const cleanup of term.cleanupHandlers || []) {
    try {
      if (typeof cleanup === 'function') cleanup();
      else if (cleanup && typeof cleanup.dispose === 'function') cleanup.dispose();
    } catch (error) {
      console.error(`Failed to clean up terminal ${term.id}:`, error);
    }
  }
  term.cleanupHandlers = [];
  if (dispose) {
    safelyDisposeTerminal(term);
  }
}

// Ids closed while their createTerminalInstance was still awaiting the
// backend: the tail of that call must not re-register them.
const closedDuringInit = new Set();

function finalizeTerminalRemoval(id) {
  closedDuringInit.add(id);
  setTimeout(() => closedDuringInit.delete(id), 60000);
  stopAgentWatch(id);
  const existing = get(terminals).find(t => t.id === id);
  cleanupTerminalResources(existing, { dispose: false });
  const nextTerminals = get(terminals).filter(t => t.id !== id);
  terminals.set(nextTerminals);
  layoutTree.set(removeLeafFromTree(get(layoutTree), id));

  if (get(activeTerminalId) === id) {
    const visibleIds = get(layoutTree) ? collectLeafIds(get(layoutTree)) : [];
    const nextActive =
      nextTerminals.find((terminal) => visibleIds.includes(terminal.id)) ||
      nextTerminals[0] ||
      null;
    activeTerminalId.set(nextActive ? nextActive.id : null);
  }
  // Closing a pane with the mouse left focus on <body>; hand it to the pane
  // that became active.
  setTimeout(() => focusTerminal(get(activeTerminalId)), 0);

  RemoveTerminalState(id);
  scheduleSessionSave();
}

export async function createTerminalInstance(id, type, name, minimized, sshProfileId = '', restoring = false, existingTmuxSessionName = '', existingResumeId = '', initialRestoreClass = 'fresh') {
  const terminal = new Terminal({
    cursorBlink: true,
    cursorStyle: 'bar',
    fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
    fontSize: get(terminalFontSize),
    lineHeight: 1.35,
    scrollback: 100000,
    // Also sets the width of xterm 6's scrollbar slider (default 14px).
    overviewRuler: { width: 8 },
    theme: XTERM_THEME
  });
  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);
  // Unicode 11 widths: without this, emoji and newer symbols (spinners,
  // status glyphs of agent TUIs) are measured as one cell and overlap the
  // next character.
  try {
    terminal.loadAddon(new Unicode11Addon());
    terminal.unicode.activeVersion = '11';
  } catch (error) {
    console.warn('Unicode 11 widths unavailable:', error?.message || error);
  }
  const searchAddon = new SearchAddon();
  terminal.loadAddon(searchAddon);
  terminal.loadAddon(new ClipboardAddon(undefined, createWriteOnlyClipboardProvider()));
  const linkProviderDisposable = terminal.registerLinkProvider(createLinkProvider(id, terminal));
  // Keep Mimir's global shortcuts out of the shell; the window handler still
  // receives them because xterm does not stop propagation. The default action
  // is cancelled here as well: when xterm skips a key it leaves the browser's
  // default in place, and WebKitGTK then inserts the character into xterm's
  // textarea, whose input event xterm forwards to the PTY — Ctrl+Shift+M
  // ended up typing "M" into Claude Code on Linux.
  terminal.attachCustomKeyEventHandler((event) => {
    // Escape closes Mimir's topmost overlay (context menu, search bar,
    // pickers) instead of reaching the shell; without an overlay it is the
    // shell's key as before. xterm stops propagation of handled keys, so
    // this is the only place that sees Escape while the terminal has focus.
    if (event.key === 'Escape' && event.type === 'keydown' && closeTopOverlay()) {
      event.preventDefault?.();
      event.stopPropagation?.();
      return false;
    }
    if (isCopyShortcut(event) && terminal.hasSelection()) {
      // Ctrl+Shift+C / Ctrl+Insert copy the selection (also one made with
      // Shift+drag in a pane whose program owns the mouse). Only on keydown,
      // and only when something is selected: otherwise the key goes to the
      // shell as usual.
      if (event.type === 'keydown') {
        const text = terminal.getSelection();
        if (text) ClipboardSetText(text).catch((error) => console.error('Copy failed:', error));
      }
      event.preventDefault?.();
      return false;
    }
    if (!isGlobalShortcut(event)) return true;
    event.preventDefault?.();
    return false;
  });

  const newTerminal = {
    id,
    terminal,
    fitAddon,
    searchAddon,
    minimized,
    name,
    editingName: false,
    type,
    outputBuffer: '',
    sshProfileId,
    disconnected: false,
    reconnecting: false,
    searchVisible: false,
    searchQuery: '',
    tmuxSessionName: '',
    tmuxOwned: false,
    tmuxActive: false,
    tmuxMode: '',
    tmuxStatus: '',
    tmuxError: '',
    rcMode: '',
    rcStatus: '',
    shellPath: '',
    resumeId: existingResumeId || generateResumeId(),
    restoreClass: initialRestoreClass,
    restoredTranscript: '',
    restoreDismissed: false,
    recording: false,
    folderId: '',
    cleanupHandlers: []
  };
  terminals.update(list => [...list, newTerminal]);

  saveTranscriptMetadata({
    resumeId: newTerminal.resumeId,
    name: newTerminal.name,
    type: newTerminal.type,
    sshProfileId: newTerminal.sshProfileId,
  });

  await tick();

  // Input and title handlers do not need a DOM element, so they are wired
  // unconditionally. Doing this only when the element existed at creation
  // left terminals that were created or restored minimized without any
  // keyboard path: after un-minimizing they rendered but never reacted.
  const inputDisposable = terminal.onData(data => {
    WriteToTerminal(id, data);
  });
  newTerminal.cleanupHandlers.push(inputDisposable);

  // Agents such as Claude Code report their state through the window
  // title; feed it to the agent detector (see agentActions.js).
  const titleDisposable = terminal.onTitleChange(title => handleTerminalTitle(id, title));
  newTerminal.cleanupHandlers.push(titleDisposable);
  newTerminal.cleanupHandlers.push(() => wiredDom.delete(id));
  newTerminal.cleanupHandlers.push(() => clearHoveredLink(id));
  newTerminal.cleanupHandlers.push(() => forgetTerminalResize(id));
  newTerminal.cleanupHandlers.push(linkProviderDisposable);

  const element = document.getElementById(`terminal-${id}`);
  if (element) {
    if (safelyAttachTerminal(newTerminal, element, { renderer: get(terminalRenderer), WebglAddon })) {
      try {
        terminal.focus();
      } catch (error) {
        console.error(`Failed to focus terminal ${id}:`, error);
      }
      safelyWriteTerminal(newTerminal, '\x1b[2J\x1b[H');
      safelyFitAndResizeTerminal(newTerminal, ResizeTerminal);
      wireTerminalDom(newTerminal);
    }
  }
  // No pane element yet (another page is showing): the events, the ready
  // handshake and the agent watch are wired like for a minimized terminal,
  // and the next reinitializeTerminals() attaches the xterm when the
  // Terminals page renders. Returning early here used to leave a dead,
  // persisted pane that never printed anything.

  const offOutput = EventsOn(`terminal-output-${id}`, data => {
    safelyWriteTerminal(newTerminal, data);
    noteTerminalOutput(id, data);
    terminals.update(list => list.map(t => {
      if (t.id !== id) return t;
      const nextOutput = (t.outputBuffer + data).slice(-12000);
      return { ...t, outputBuffer: nextOutput };
    }));
    appendTerminalTranscript(newTerminal.resumeId, data);
  });
  newTerminal.cleanupHandlers.push(offOutput);

  const offClosed = EventsOn(`terminal-closed-${id}`, () => {
    const term = get(terminals).find(t => t.id === id);
    if (term) {
      cleanupTerminalResources(term);
    }
    finalizeTerminalRemoval(id);
    reinitializeTerminals();
  });
  newTerminal.cleanupHandlers.push(offClosed);

  const offPrompt = EventsOn(`terminal-prompt-${id}`, () => handleTerminalPrompt(id));
  newTerminal.cleanupHandlers.push(offPrompt);

  const offDisconnected = EventsOn(`terminal-disconnected-${id}`, () => {
    terminals.update(list => list.map(t => {
      if (t.id !== id) return t;
      return { ...t, disconnected: true, reconnecting: false };
    }));
  });
  newTerminal.cleanupHandlers.push(offDisconnected);

  await ConfirmFrontendReady(id);
  if (closedDuringInit.has(id)) { cleanupTerminalResources(newTerminal); return null; }
  await InitializeTerminal(id);
  if (closedDuringInit.has(id)) { cleanupTerminalResources(newTerminal); return null; }

  startAgentWatch(id, type);
  newTerminal.cleanupHandlers.push(() => stopAgentWatch(id));

  try {
    const status = await readTmuxStatus(id);
    terminals.update(list => list.map((t) => {
      if (t.id !== id) return t;
      return {
        ...t,
        tmuxActive: Boolean(status?.active),
        tmuxSessionName: status?.sessionName || existingTmuxSessionName || '',
        tmuxMode: status?.mode || '',
        tmuxStatus: status?.status || '',
        tmuxError: status?.error || '',
        tmuxVersion: status?.version || '',
        tmuxMouse: Boolean(status?.mouse),
        rcMode: status?.rcMode || '',
        rcStatus: status?.rcStatus || '',
        shellPath: status?.shellPath || '',
        tmuxOwned: Boolean(status?.active) && type !== 'ssh' && !restoring
      };
    }));
    newTerminal.tmuxActive = Boolean(status?.active);
    newTerminal.tmuxSessionName = status?.sessionName || existingTmuxSessionName || '';
    newTerminal.tmuxMode = status?.mode || '';
    newTerminal.tmuxStatus = status?.status || '';
    newTerminal.tmuxError = status?.error || '';
    newTerminal.tmuxVersion = status?.version || '';
    newTerminal.tmuxMouse = Boolean(status?.mouse);
    newTerminal.rcMode = status?.rcMode || '';
    newTerminal.rcStatus = status?.rcStatus || '';
    newTerminal.shellPath = status?.shellPath || '';
    newTerminal.tmuxOwned = Boolean(status?.active) && type !== 'ssh' && !restoring;
  } catch (error) {
    console.error(`Failed to read tmux status for terminal ${id}:`, error);
  }

  if (!tmuxCapableTerminalTypes.has(type) && type !== 'ssh') {
    await WriteToTerminal(id, '\r');
    // PowerShell gets its prompt from the backend profile (so the cwd /
    // history beacon is never replaced by a prompt typed here). cmd has no
    // profile; its short prompt is typed only in Mimir prompt mode.
    if (type === 'cmd' && get(promptMode) === 'mimir') {
      await WriteToTerminal(id, 'prompt %USERNAME% $G$S& cls\r');
    }
  }

  return newTerminal;
}

// Terminal ids whose DOM-dependent handlers (resize observer, paste) are
// installed. Kept outside the terminal objects because the store replaces
// those objects on every output chunk.
const wiredDom = new Set();

// wireTerminalDom installs the handlers that need xterm's rendered element.
// It runs on the first successful attach, which for terminals created or
// restored minimized happens when they are first shown, not at creation.
// xterm's own element survives re-attaches, so listening there (rather than
// on the Svelte container) keeps the handlers valid across layout changes.
export function wireTerminalDom(term) {
  if (!term?.terminal?.element || wiredDom.has(term.id)) return;
  wiredDom.add(term.id);

  // Refit whenever the terminal's rendered box changes size (layout-tree
  // changes, split-drag, window resize). Keeps cols/rows in sync with the
  // pane so content reflows to the full width instead of staying wrapped at
  // a previous, narrower size.
  const disposeResizeObserver = observeTerminalResize(term, ResizeTerminal);
  term.cleanupHandlers.push(disposeResizeObserver);

  const xtermElement = term.terminal.element;
  const handlePaste = (event) => {
    const pasteData = event.clipboardData?.getData('text');
    if (pasteData) {
      // Route through xterm so bracketed paste applies; otherwise multiline
      // clipboard content executes line by line the moment it is pasted.
      term.terminal.paste(pasteData);
    }
    event.preventDefault();
  };
  xtermElement.addEventListener('paste', handlePaste);
  term.cleanupHandlers.push(() => xtermElement.removeEventListener('paste', handlePaste));
}

function isCopyShortcut(event) {
  if (event.ctrlKey && event.shiftKey && !event.altKey && (event.key === 'C' || event.key === 'c')) return true;
  return event.ctrlKey && !event.shiftKey && !event.altKey && event.key === 'Insert';
}

/**
 * Re-reads the tmux status of every terminal (after the tmux integration
 * mode changed, which is applied to running sessions out-of-band).
 */
export async function refreshTmuxStatuses() {
  for (const t of get(terminals)) {
    try {
      const status = await readTmuxStatus(t.id);
      if (!status) continue;
      terminals.update(list => list.map((x) => x.id !== t.id ? x : {
        ...x,
        tmuxActive: Boolean(status.active),
        tmuxSessionName: status.sessionName || x.tmuxSessionName || '',
        tmuxMode: status.mode || '',
        tmuxStatus: status.status || '',
        tmuxError: status.error || '',
        tmuxVersion: status.version || '',
        tmuxMouse: Boolean(status.mouse),
      }));
    } catch (error) {
      console.error(`Failed to refresh tmux status for terminal ${t.id}:`, error);
    }
  }
}

export async function reinitializeTerminals() {
  await tick();
  for (const t of get(terminals)) {
    if (!t.minimized) {
      const element = document.getElementById(`terminal-${t.id}`);
      if (element) {
        if (safelyAttachTerminal(t, element, { renderer: get(terminalRenderer), WebglAddon })) {
          wireTerminalDom(t);
          // The container is new after a layout change: watch it, then fit.
          rebindTerminalResize(t);
          safelyFitAndResizeTerminal(t, ResizeTerminal);
        }
      }
    }
  }
}

export function handleResize() {
  requestAnimationFrame(() => {
    get(terminals).forEach(t => {
      if (!t.minimized) {
        safelyFitAndResizeTerminal(t, ResizeTerminal);
      }
    });
  });
}

export async function addTerminal(terminalTypeParam, nameParam, minimized = false, initialPath = '', { selectedTerminalType, openSSHProfilePicker } = {}) {
  const type = typeof terminalTypeParam === 'string' ? terminalTypeParam : selectedTerminalType;

  if (type === 'ssh') {
    openSSHProfilePicker();
    return;
  }

  const name = typeof nameParam === 'string' ? nameParam : `${type.toUpperCase()} ${get(terminals).length + 1}`;

  // A new terminal belongs on the Terminals page; switch there first so
  // the pane element exists when the xterm is created.
  if (!minimized && get(currentPage) !== 'terminals') {
    currentPage.set('terminals');
    await tick();
  }

  try {
    const tmuxSessionName = tmuxCapableTerminalTypes.has(type) ? generateTmuxSessionName('mimir') : '';
    const id = await startTerminalBackend(type, tmuxSessionName);
    if (!id) {
      errorMessage.set("Failed to start terminal. The backend returned an invalid ID.");
      return;
    }

    const newLeaf = { type: 'leaf', terminalId: id };
    if (!minimized) {
      // Equal shares for every top-level pane (see appendLeaf).
      layoutTree.set(appendLeaf(get(layoutTree), newLeaf));
    }

    const newTerminal = await createTerminalInstance(id, type, name, minimized, '', false, tmuxSessionName, '', 'fresh');
    if (!newTerminal || closedDuringInit.has(id)) return; // closed while starting
    activeTerminalId.set(id);
    persistTerminalState(newTerminal);

    await reinitializeTerminals();

    if (initialPath && containsControlChars(initialPath)) {
      // Never type control characters into a fresh PTY (see handleRemoteCD).
      console.warn('Ignoring initial path with control characters');
      initialPath = '';
    }
    if (initialPath) {
      let cdCommand = '';
      switch(type) {
        case 'cmd':
          cdCommand = `cd /d "${initialPath}"`;
          break;
        case 'powershell':
          cdCommand = `Set-Location -LiteralPath "${initialPath}"`;
          break;
        case 'wsl':
        case 'bash':
        case 'zsh':
          cdCommand = `cd ${shellQuotePath(initialPath)}`;
          break;
      }
      if (cdCommand) {
        await WriteToTerminal(id, cdCommand + '\r');
      }
    }
  } catch (error) {
    const msg = error instanceof Error ? error.message : String(error);
    errorMessage.set(msg);
    console.error(`addTerminal: ${msg}`, error);
  }
}

export async function splitTerminal(terminalId, direction) {
  const sourceTerm = get(terminals).find(t => t.id === terminalId);
  if (!sourceTerm) return;

  const type = sourceTerm.type;

  try {
    let newId;
    let name;
    let sshProfileId = '';

    if (type === 'ssh' && sourceTerm.sshProfileId) {
      const profile = get(sshProfiles).find(p => p.id === sourceTerm.sshProfileId);
      if (!profile) {
        errorMessage.set('SSH profile not found. Cannot split this terminal.');
        return;
      }
      newId = await StartSSHTerminal(profile.id);
      name = `SSH: ${profile.name}`;
      sshProfileId = profile.id;
    } else {
      const tmuxSessionName = tmuxCapableTerminalTypes.has(type) ? generateTmuxSessionName('mimir') : '';
      newId = await startTerminalBackend(type, tmuxSessionName);
      name = `${type.toUpperCase()} ${get(terminals).length + 1}`;
    }
    if (!newId) return;

    const newLeaf = { type: 'leaf', terminalId: newId };
    layoutTree.set(replaceLeaf(get(layoutTree), terminalId, {
      type: 'split',
      direction: direction,
      ratio: 0.5,
      children: [
        { type: 'leaf', terminalId: terminalId },
        newLeaf
      ]
    }));

    const newTerminal = await createTerminalInstance(newId, type, name, false, sshProfileId, false, '', '', 'fresh');
    if (!newTerminal || closedDuringInit.has(newId)) return; // closed while starting
    persistTerminalState({ ...newTerminal, minimized: false, sshProfileId });
    activeTerminalId.set(newId);

    await reinitializeTerminals();
  } catch (error) {
    errorMessage.set(error.message || 'Failed to split terminal.');
  }
}

export async function toggleRecording(terminalId) {
  const term = get(terminals).find(t => t.id === terminalId);
  if (!term) return;

  const next = !term.recording;
  try {
    if (next) {
      await StartRecording(terminalId, term.name || `Terminal ${terminalId}`);
    } else {
      await StopRecording(terminalId);
    }
    // The terminal object is replaced on every output chunk; set the flag
    // on the current one, never on the copy captured before the await.
    terminals.update(list => list.map(t => (t.id === terminalId ? { ...t, recording: next } : t)));
  } catch (e) {
    console.error('Recording toggle failed:', e);
    errorMessage.set(`Recording: ${e?.message || e}`);
  }
}

export function removeTerminal(id) {
  const term = get(terminals).find(t => t.id === id);
  if (term?.tmuxSessionName && term?.tmuxOwned) {
    KillTmuxSession(term.tmuxSessionName).catch(() => {});
  }
  if (term && term.type === 'ssh' && term.disconnected) {
    CloseSSHTerminalFull(id);
    safelyDisposeTerminal(term, 'disconnected terminal');
    finalizeTerminalRemoval(id);
    reinitializeTerminals();
    return;
  }
  CloseTerminal(id);
  setTimeout(() => {
    const stillExists = get(terminals).find(t => t.id === id);
    if (stillExists) {
      safelyDisposeTerminal(stillExists, 'fallback terminal');
      finalizeTerminalRemoval(id);
      reinitializeTerminals();
    }
  }, 500);
}

export async function reconnectSSH(id) {
  terminals.set(markReconnectStarted(get(terminals), id));
  try {
    await ReconnectSSHTerminal(id);
    await ConfirmFrontendReady(id);
    await InitializeTerminal(id);
    terminals.set(markReconnectSucceeded(get(terminals), id, safelyWriteTerminal));
  } catch (e) {
    terminals.set(markReconnectFailed(get(terminals), id, e, safelyWriteTerminal));
  }
}

export async function terminalToBackground(id) {
  terminals.update(list => list.map(t => {
    if (t.id === id) {
      const next = { ...t, minimized: true };
      persistTerminalState(next);
      return next;
    }
    return t;
  }));
  layoutTree.set(removeLeafFromTree(get(layoutTree), id));
  // The keyboard must not stay on a pane that just left the screen.
  if (get(activeTerminalId) === id) {
    const visible = get(layoutTree) ? collectLeafIds(get(layoutTree)) : [];
    activeTerminalId.set(visible.length ? visible[0] : null);
  }
  await reinitializeTerminals();
  focusTerminal(get(activeTerminalId));
}

export async function terminalToForeground(id) {
  terminals.update(list => list.map(t => {
    if (t.id === id) {
      const next = { ...t, minimized: false };
      persistTerminalState(next);
      return next;
    }
    return t;
  }));

  const newLeaf = { type: 'leaf', terminalId: id };
  layoutTree.set(appendLeaf(get(layoutTree), newLeaf));

  await reinitializeTerminals();
}

export async function toggleMinimize(id) {
  const term = get(terminals).find(t => t.id === id);
  if (term) {
    if (term.minimized) {
      await terminalToForeground(id);
    } else {
      await terminalToBackground(id);
    }
  }
}

export function startEditingName(id) {
  terminals.update(list => list.map(t => {
    if (t.id === id) return { ...t, editingName: true };
    return t;
  }));
  tick().then(() => {
    const el = document.getElementById(`terminal-name-input-${id}`);
    if (el) el.focus();
  });
}

export function saveTerminalName(id, event) {
  terminals.update(list => list.map(t => {
    if (t.id === id) {
      const newName = event.target.value;
      const next = { ...t, name: newName, editingName: false };
      persistTerminalState(next);
      if (next.name !== t.name) {
        saveTranscriptMetadata({
          resumeId: next.resumeId,
          name: next.name,
          type: next.type,
          sshProfileId: next.sshProfileId,
        });
      }
      return next;
    }
    return t;
  }));
}

// Zoom: apply a changed font size to every open terminal and re-fit them
// so the shell learns the new rows × cols.
terminalFontSize.subscribe((size) => {
  for (const term of get(terminals)) {
    if (!term?.terminal) continue;
    try {
      if (term.terminal.options.fontSize !== size) term.terminal.options.fontSize = size;
      safelyFitAndResizeTerminal(term, ResizeTerminal);
    } catch (error) {
      console.error(`Failed to apply font size to terminal ${term.id}:`, error);
    }
  }
});

// Renderer switch applies live to every open terminal.
terminalRenderer.subscribe((mode) => {
  for (const term of get(terminals)) {
    if (!term?.terminal?.element) continue;
    if (mode === 'dom') disableWebgl(term);
    else enableWebgl(term, WebglAddon);
  }
});

// The bundled font may finish loading after the first terminals measured
// their cells; re-measure once it is ready so columns line up.
export function refreshTerminalFonts() {
  for (const term of get(terminals)) {
    if (!term?.terminal) continue;
    try {
      const family = term.terminal.options.fontFamily;
      term.terminal.options.fontFamily = family + ' ';
      term.terminal.options.fontFamily = family;
      safelyFitAndResizeTerminal(term, ResizeTerminal);
    } catch (error) {
      console.error(`Failed to refresh font of terminal ${term.id}:`, error);
    }
  }
}
try {
  if (typeof document !== 'undefined' && document.fonts?.ready) {
    document.fonts.ready.then(() => refreshTerminalFonts()).catch(() => {});
  }
} catch { /* no Font Loading API */ }
