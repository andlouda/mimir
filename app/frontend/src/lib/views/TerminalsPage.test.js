// @vitest-environment jsdom
import { afterEach, beforeEach, expect, test } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import { get } from 'svelte/store';
import TerminalsPage from './TerminalsPage.svelte';
import { activeTerminalId, availableTerminalTypes, layoutTree, selectedTerminalType, terminals } from '../stores/terminalStore.js';
import { errorMessage, showAIMenu } from '../stores/uiStore.js';
import { aiPanelState } from '../stores/aiStore.js';
import { DEFAULT_TERMINAL_TYPE_KEY } from '../terminals/terminalTypes.js';
import { installBackendStub, removeBackendStub } from '../../test/backendStub.js';

// Header of the terminals page without any pane: the type picker, the AI
// menu and the empty state. Panes themselves need xterm and are covered
// by the Playwright suite.

let view;

beforeEach(() => {
  installBackendStub();
  localStorage.clear();
  availableTerminalTypes.set([{ value: 'bash', label: 'Bash' }, { value: 'zsh', label: 'Zsh' }, { value: 'ssh', label: 'SSH' }]);
  selectedTerminalType.set('bash');
  terminals.set([]);
  activeTerminalId.set(null);
  layoutTree.set(null);
  showAIMenu.set(false);
  aiPanelState.set(null);
  errorMessage.set('');
  view = mount(TerminalsPage, { target: document.body });
  flushSync();
});

afterEach(() => {
  unmount(view);
  document.body.innerHTML = '';
  removeBackendStub();
});

test('shows the empty state and the offered terminal types', () => {
  expect(document.body.textContent).toContain('No terminals open');
  const options = [...document.querySelectorAll('.terminal-controls select option')].map((o) => o.value);
  expect(options).toEqual(['bash', 'zsh', 'ssh']);
  expect(document.querySelector('.terminal-controls select').value).toBe('bash');
});

test('changing the picker updates the store and remembers the default', () => {
  const select = document.querySelector('.terminal-controls select');
  select.value = 'zsh';
  select.dispatchEvent(new Event('change', { bubbles: true }));
  flushSync();
  expect(get(selectedTerminalType)).toBe('zsh');
  expect(localStorage.getItem(DEFAULT_TERMINAL_TYPE_KEY)).toBe('zsh');
});

test('the AI menu opens from the store and refuses to act without a terminal', () => {
  expect(document.querySelector('.ai-menu')).toBeNull();
  document.querySelector('.ai-btn').click();
  flushSync();
  expect(get(showAIMenu)).toBe(true);
  const items = document.querySelectorAll('.ai-menu-item');
  expect(items).toHaveLength(4);
  items[0].click();
  flushSync();
  expect(get(aiPanelState)).toBeNull();
  expect(get(errorMessage)).toBe('Please select a terminal first.');
});

test('with an active terminal the AI menu opens the panel for it', () => {
  terminals.set([{ id: 7, type: 'bash', name: 'BASH 1' }]);
  activeTerminalId.set(7);
  showAIMenu.set(true);
  flushSync();
  document.querySelectorAll('.ai-menu-item')[1].click();
  flushSync();
  expect(get(aiPanelState)).toMatchObject({ mode: 'suggest_next_command', terminalId: 7, terminalType: 'bash', terminalName: 'BASH 1' });
  expect(get(showAIMenu)).toBe(false);
});
