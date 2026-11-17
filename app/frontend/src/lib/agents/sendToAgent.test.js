import { beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
import { agentStates } from '../stores/agentStore.js';
import { agentTargets, pasteTargets, pasteUnmaskedFromAgent, sendMaskedTextToAgent, sendTextToAgent } from './sendToAgent.js';

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

  test('masked send pastes what the backend returns, never the original', async () => {
    const mask = vi.fn(async () => JSON.stringify({ text: 'db MIMIR_IP_1 down', masked: { IP: 1 } }));
    const result = await sendMaskedTextToAgent(2, 'db 10.0.3.17 down\n', { mask });
    expect(mask).toHaveBeenCalledWith(2, 'db 10.0.3.17 down\n');
    expect(paste).toHaveBeenCalledWith('db MIMIR_IP_1 down');
    expect(result).toEqual({ ok: true, masked: { IP: 1 } });
  });

  test('masked send fails closed without a masking backend', async () => {
    expect(await sendMaskedTextToAgent(2, 'db 10.0.3.17 down', { mask: undefined })).toEqual({ ok: false, masked: {} });
    expect(paste).not.toHaveBeenCalled();
  });

  test('a command from the agent is translated back and pasted into the chosen pane', async () => {
    const targetPaste = get(terminals)[0].terminal.paste;
    const unmask = vi.fn(async () => JSON.stringify({ text: 'ssh root@10.0.3.17', restored: 1 }));
    const result = await pasteUnmaskedFromAgent(2, 1, 'ssh root@MIMIR_IP_1\n', { unmask });
    expect(unmask).toHaveBeenCalledWith(2, 'ssh root@MIMIR_IP_1');
    expect(targetPaste).toHaveBeenCalledWith('ssh root@10.0.3.17');
    expect(result).toEqual({ ok: true, restored: 1, unknown: [] });
    expect(get(activeTerminalId)).toBe(1);
  });

  test('a command with an unknown placeholder is not pasted', async () => {
    const targetPaste = get(terminals)[0].terminal.paste;
    const unmask = vi.fn(async () => JSON.stringify({ text: 'ssh MIMIR_IP_9', restored: 0, unknown: ['MIMIR_IP_9'] }));
    const result = await pasteUnmaskedFromAgent(2, 1, 'ssh MIMIR_IP_9', { unmask });
    expect(result).toEqual({ ok: false, restored: 0, unknown: ['MIMIR_IP_9'] });
    expect(targetPaste).not.toHaveBeenCalled();
  });

  test('paste targets are the other visible panes', () => {
    expect(pasteTargets(2)).toEqual([{ id: 1, name: 'BASH 1' }]);
  });
});
