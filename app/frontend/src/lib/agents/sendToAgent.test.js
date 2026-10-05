import { beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
import { agentStates } from '../stores/agentStore.js';
import { agentTargets, sendTextToAgent } from './sendToAgent.js';

describe('send to agent', () => {
  let paste;
  beforeEach(() => {
    paste = vi.fn();
    terminals.set([
      { id: 1, name: 'BASH 1', terminal: { paste: vi.fn(), focus: vi.fn() } },
      { id: 2, name: 'BASH 2', terminal: { paste, focus: vi.fn() } },
      { id: 3, name: 'min', minimized: true, terminal: { paste: vi.fn() } },
    ]);
    agentStates.set({ 2: { kind: 'claude', label: 'Claude', short: 'C' }, 3: { kind: 'codex', label: 'Codex', short: 'cx' } });
    activeTerminalId.set(1);
  });

  test('lists agent panes except the source', () => {
    expect(agentTargets(1).map((t) => t.id)).toEqual([2, 3]);
    expect(agentTargets(2).map((t) => t.id)).toEqual([3]);
    expect(agentTargets(1)[0]).toMatchObject({ label: 'C', name: 'BASH 2' });
  });

  test('pastes without a trailing newline and activates the pane', () => {
    expect(sendTextToAgent(2, 'ls -la\n\n')).toBe(true);
    expect(paste).toHaveBeenCalledWith('ls -la');
    expect(get(activeTerminalId)).toBe(2);
    expect(sendTextToAgent(2, '   \n')).toBe(false);
    expect(sendTextToAgent(3, 'x')).toBe(false); // minimized pane cannot take input
    expect(sendTextToAgent(9, 'x')).toBe(false);
  });
});
