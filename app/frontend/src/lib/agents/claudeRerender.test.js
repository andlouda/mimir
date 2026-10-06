import { describe, expect, test, vi } from 'vitest';
import { createClaudeRerender, WIDEN_COLS } from './claudeRerender.js';

function setup(agent, opts = {}) {
  const run = vi.fn(async () => 'claude --resume x');
  const results = [];
  const r = createClaudeRerender({ run, getAgent: () => agent, onResult: (id, res) => results.push(res), ...opts });
  return { r, run, results };
}

describe('Claude rerender after a widen', () => {
  test('runs right away when Claude is idle and the pane grew enough', async () => {
    const { r, run, results } = setup({ kind: 'claude', status: 'idle', prompt: '' });
    expect(r.onResized(1, 100, 100 + WIDEN_COLS)).toBe(true);
    await Promise.resolve();
    expect(run).toHaveBeenCalledWith(1);
    await vi.waitFor(() => expect(results[0]).toEqual({ ok: true, command: 'claude --resume x' }));
  });

  test('ignores small widens, narrowing, other agents and a disabled setting', () => {
    expect(setup({ kind: 'claude', status: 'idle' }).r.onResized(1, 100, 100 + WIDEN_COLS - 1)).toBe(false);
    expect(setup({ kind: 'claude', status: 'idle' }).r.onResized(1, 140, 100)).toBe(false);
    expect(setup({ kind: 'codex', status: 'idle' }).r.onResized(1, 100, 160)).toBe(false);
    expect(setup({ kind: 'claude', status: 'idle' }, { isEnabled: () => false }).r.onResized(1, 100, 160)).toBe(false);
  });

  test('waits for idle while Claude works or asks for an approval', async () => {
    const agent = { kind: 'claude', status: 'working', prompt: '' };
    const { r, run } = setup(agent);
    expect(r.onResized(1, 100, 160)).toBe(true);
    expect(run).not.toHaveBeenCalled();
    r.onAgentState(1, 'permission', 'permission_prompt');
    expect(run).not.toHaveBeenCalled();
    r.onAgentState(1, 'idle', '');
    await Promise.resolve();
    expect(run).toHaveBeenCalledTimes(1);
    r.onAgentState(1, 'idle', ''); // once per widen
    expect(run).toHaveBeenCalledTimes(1);
  });

  test('reports a failed restart', async () => {
    const results = [];
    const r = createClaudeRerender({ run: async () => { throw new Error('Claude did not exit'); }, getAgent: () => ({ kind: 'claude', status: 'idle' }), onResult: (id, res) => results.push(res) });
    await r.trigger(1);
    expect(results[0]).toEqual({ ok: false, error: 'Claude did not exit' });
  });
});
