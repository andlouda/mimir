// Claude Code hard-wraps its finished output at the pane width of the
// moment; after the pane is widened again the old lines stay narrow and
// no terminal can re-wrap them. The one remedy is a restart with
// --resume, which prints the conversation again at the current width.
// This module decides when Mimir asks the backend to do that: after a
// widen of at least WIDEN_COLS columns, once Claude is idle (never while
// it answers or waits for an approval), at most once per widen.

export const WIDEN_COLS = 20;

// Idle means Claude waits for the user. The hook reports that as
// "idle_prompt", which is fine; only a pending permission_prompt blocks.
export function isIdle(status, prompt) {
  return status === 'idle' && prompt !== 'permission_prompt';
}

export function createClaudeRerender({ run, isEnabled = () => true, getAgent, onResult = () => {} }) {
  const pending = new Set(); // terminal ids waiting for idle
  const running = new Set();

  function wants(id, prevCols, cols) {
    if (!isEnabled()) return false;
    const agent = getAgent(id);
    if (!agent || agent.kind !== 'claude') return false;
    return cols - prevCols >= WIDEN_COLS;
  }

  /** Called after a resize went to the PTY. */
  function onResized(id, prevCols, cols) {
    if (!wants(id, prevCols, cols)) return false;
    const agent = getAgent(id);
    if (isIdle(agent.status, agent.prompt)) {
      pending.delete(id);
      trigger(id);
    } else {
      pending.add(id); // when the answer or the approval is done
    }
    return true;
  }

  /** Called on every agent state change. */
  function onAgentState(id, status, prompt) {
    if (!pending.has(id)) return;
    if (isIdle(status, prompt)) {
      pending.delete(id);
      trigger(id);
    }
  }

  function forget(id) { pending.delete(id); running.delete(id); }

  async function trigger(id) {
    if (running.has(id)) return;
    running.add(id);
    try {
      const command = await run(id);
      onResult(id, { ok: true, command });
    } catch (error) {
      onResult(id, { ok: false, error: error?.message || String(error) });
    } finally {
      running.delete(id);
    }
  }

  return { onResized, onAgentState, forget, trigger, isPending: (id) => pending.has(id) };
}
