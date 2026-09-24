// Live state is deliberately separate from persisted task state. A vanished
// process or an idle agent never marks a user's task as completed.
export function liveWorkspaceSessions(states, terminals, workspace) {
  const terms = new Map(terminals.map((term) => [term.id, term]));
  return Object.entries(states).flatMap(([id, agent]) => {
    const terminal = terms.get(Number(id));
    if (!terminal) return [];
    // SSH needs the resolved host; a generic "ssh" source is not an identity.
    const host = agent.project?.host || (agent.source === 'ssh' ? '' : agent.source || 'local');
    const session = agent.sessionFile && host ? workspace.sessions.find((ref) =>
      ref.host === host && ref.kind === agent.kind && ref.file === agent.sessionFile) : null;
    return [{ id: Number(id), agent, terminal, session: session || null }];
  });
}

export function workspaceAttention(workspace, live) {
  const rows = live.flatMap((row) => {
    const reason = row.agent.status === 'permission' ? 'permission'
      : row.agent.status === 'idle' ? 'input'
      : row.agent.attention && row.agent.status !== 'working' ? 'result' : '';
    return reason ? [{ ...row, key: `agent:${row.id}`, reason, projectId: row.session?.projectId || '', taskId: row.session?.taskId || '' }] : [];
  });
  for (const task of workspace.tasks) {
    const project = workspace.projects.find((p) => p.id === task.projectId);
    if (task.archived || !project || project.archived || !['blocked', 'review'].includes(task.status)) continue;
    rows.push({ key: `task:${task.id}`, reason: task.status, task, projectId: task.projectId, taskId: task.id });
  }
  const priority = { permission: 0, blocked: 1, input: 2, result: 3, review: 4 };
  return rows.sort((a, b) => priority[a.reason] - priority[b.reason]);
}
