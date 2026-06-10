import { get } from 'svelte/store';
import { tick } from 'svelte';
import { terminals, activeTerminalId, layoutTree } from '../stores/terminalStore.js';
import { errorMessage } from '../stores/uiStore.js';
import { sshProfiles } from '../stores/sshStore.js';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { SearchAddon } from '@xterm/addon-search';
import { ClipboardAddon } from '@xterm/addon-clipboard';
import { createWriteOnlyClipboardProvider } from '../terminals/osc52Clipboard.js';
import { EventsOn } from '../../../wailsjs/runtime';
import { WriteToTerminal, ResizeTerminal, CloseTerminal, InitializeTerminal, ConfirmFrontendReady, StartTerminal, StartSSHTerminal, CloseSSHTerminalFull, KillTmuxSession, StartRecording, StopRecording, RemoveTerminalState, ReconnectSSHTerminal } from '../../../wailsjs/go/main/App';
import { replaceLeaf, removeLeafFromTree, collectLeafIds } from '../terminals/layoutTree.js';
import { generateTmuxSessionName } from '../terminals/tmuxLifecycle.js';
import { generateResumeId, shellQuotePath } from '../util.js';
import { safelyWriteTerminal, safelyFitAndResizeTerminal, safelyAttachTerminal, safelyDisposeTerminal } from '../terminals/xtermLifecycle.js';
import { markReconnectStarted, markReconnectSucceeded, markReconnectFailed } from '../terminals/reconnectLifecycle.js';
import { appendTerminalTranscript, saveTranscriptMetadata } from '../transcript/transcriptApi.js';
import { persistTerminalState, scheduleSessionSave } from './sessionActions.js';

const tmuxCapableTerminalTypes = new Set(['bash', 'zsh', 'wsl']);

const XTERM_THEME = {
  background: '#0c0e14',
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

function finalizeTerminalRemoval(id) {
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

  RemoveTerminalState(id);
  scheduleSessionSave();
}

export async function createTerminalInstance(id, type, name, minimized, sshProfileId = '', restoring = false, existingTmuxSessionName = '', existingResumeId = '', initialRestoreClass = 'fresh') {
  const terminal = new Terminal({
    cursorBlink: true,
    cursorStyle: 'bar',
    fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
    fontSize: 13,
    lineHeight: 1.35,
    scrollback: 100000,
    theme: XTERM_THEME
  });
  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);
  const searchAddon = new SearchAddon();
  terminal.loadAddon(searchAddon);
  terminal.loadAddon(new ClipboardAddon(undefined, createWriteOnlyClipboardProvider()));

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

  const element = document.getElementById(`terminal-${id}`);
  if (element) {
    if (safelyAttachTerminal(newTerminal, element)) {
      try {
        terminal.focus();
      } catch (error) {
        console.error(`Failed to focus terminal ${id}:`, error);
      }
      safelyWriteTerminal(newTerminal, '\x1b[2J\x1b[H');
      safelyFitAndResizeTerminal(newTerminal, ResizeTerminal);
    }

    const inputDisposable = terminal.onData(data => {
      WriteToTerminal(id, data);
    });
    newTerminal.cleanupHandlers.push(inputDisposable);

    const handlePaste = (event) => {
      const pasteData = event.clipboardData.getData('text');
      if (pasteData) {
        // Route through xterm so bracketed paste applies; otherwise multiline
        // clipboard content executes line by line the moment it is pasted.
        terminal.paste(pasteData);
      }
      event.preventDefault();
    };
    element.addEventListener('paste', handlePaste);
    newTerminal.cleanupHandlers.push(() => element.removeEventListener('paste', handlePaste));
  } else if (!minimized) {
    errorMessage.set(`Failed to find terminal element for ID: ${id}`);
    console.error(get(errorMessage));
    return newTerminal;
  }

  const offOutput = EventsOn(`terminal-output-${id}`, data => {
    safelyWriteTerminal(newTerminal, data);
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

  const offDisconnected = EventsOn(`terminal-disconnected-${id}`, () => {
    terminals.update(list => list.map(t => {
      if (t.id !== id) return t;
      return { ...t, disconnected: true, reconnecting: false };
    }));
  });
  newTerminal.cleanupHandlers.push(offDisconnected);

  await ConfirmFrontendReady(id);
  await InitializeTerminal(id);

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
    newTerminal.rcMode = status?.rcMode || '';
    newTerminal.rcStatus = status?.rcStatus || '';
    newTerminal.shellPath = status?.shellPath || '';
    newTerminal.tmuxOwned = Boolean(status?.active) && type !== 'ssh' && !restoring;
  } catch (error) {
    console.error(`Failed to read tmux status for terminal ${id}:`, error);
  }

  if (!tmuxCapableTerminalTypes.has(type) && type !== 'ssh') {
    await WriteToTerminal(id, '\r');
    let promptCmd = '';
    switch (type) {
      case 'powershell':
        promptCmd = 'function prompt { "$env:USERNAME ❯ " }; cls';
        break;
      case 'cmd':
        promptCmd = 'prompt %USERNAME% $G$S& cls';
        break;
    }
    if (promptCmd) {
      await WriteToTerminal(id, promptCmd + '\r');
    }
  }

  return newTerminal;
}

export async function reinitializeTerminals() {
  await tick();
  for (const t of get(terminals)) {
    if (!t.minimized) {
      const element = document.getElementById(`terminal-${t.id}`);
      if (element) {
        if (safelyAttachTerminal(t, element)) {
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

  try {
    const tmuxSessionName = tmuxCapableTerminalTypes.has(type) ? generateTmuxSessionName('mimir') : '';
    const id = await startTerminalBackend(type, tmuxSessionName);
    if (!id) {
      errorMessage.set("Failed to start terminal. The backend returned an invalid ID.");
      return;
    }

    const newLeaf = { type: 'leaf', terminalId: id };
    if (!minimized) {
      const tree = get(layoutTree);
      if (tree === null) {
        layoutTree.set(newLeaf);
      } else {
        layoutTree.set({
          type: 'split',
          direction: 'horizontal',
          ratio: 0.5,
          children: [tree, newLeaf]
        });
      }
    }

    const newTerminal = await createTerminalInstance(id, type, name, minimized, '', false, tmuxSessionName, '', 'fresh');
    activeTerminalId.set(id);
    persistTerminalState(newTerminal);

    await reinitializeTerminals();

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

  try {
    if (term.recording) {
      await StopRecording(terminalId);
      term.recording = false;
    } else {
      const name = term.name || `Terminal ${terminalId}`;
      await StartRecording(terminalId, name);
      term.recording = true;
    }
    terminals.update(list => [...list]);
  } catch (e) {
    console.error('Recording toggle failed:', e);
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
  await reinitializeTerminals();
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
  const tree = get(layoutTree);
  if (tree === null) {
    layoutTree.set(newLeaf);
  } else {
    layoutTree.set({
      type: 'split',
      direction: 'horizontal',
      ratio: 0.5,
      children: [tree, newLeaf]
    });
  }

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
