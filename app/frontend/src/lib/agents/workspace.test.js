import { describe, expect, test } from 'vitest';
import { liveWorkspaceSessions, workspaceAttention } from './workspace.js';

const data = () => ({
  version: 1,
  projects: [{ id: 'p', name: 'Mimir' }],
  tasks: [{ id: 't', projectId: 'p', status: 'in_progress' }],
  sessions: [{ id: 's', projectId: 'p', taskId: 't', host: 'local', kind: 'claude', file: '/session' }],
});

describe('workspace live associations', () => {
  test('reattaches by host, kind and session after terminal IDs change', () => {
    const workspace = data();
    const live = liveWorkspaceSessions({ 99: { kind: 'claude', source: 'local', sessionFile: '/session', status: 'idle' } }, [{ id: 99 }], workspace);
    expect(live[0].session.id).toBe('s');
    expect(workspaceAttention(workspace, live)[0]).toMatchObject({ reason: 'input', taskId: 't', id: 99 });
    expect(workspace.tasks[0].status).toBe('in_progress');
    expect(liveWorkspaceSessions({}, [], workspace)).toEqual([]);
    expect(workspace.sessions).toHaveLength(1);
  });
  test('does not attach another host, kind or unverified SSH identity', () => {
    for (const agent of [
      { kind: 'claude', source: 'ssh' },
      { kind: 'claude', project: { host: 'server' } },
      { kind: 'codex', source: 'local' },
    ]) {
      const live = liveWorkspaceSessions({ 1: { ...agent, sessionFile: '/session' } }, [{ id: 1 }], data());
      expect(live[0].session).toBeNull();
    }
  });
  test('keeps unassigned approvals visible and sorts them ahead of task reviews', () => {
    const workspace = data();
    workspace.tasks[0].status = 'review';
    const live = liveWorkspaceSessions({ 1: { kind: 'claude', status: 'permission' } }, [{ id: 1 }], workspace);
    expect(workspaceAttention(workspace, live).map((row) => row.reason)).toEqual(['permission', 'review']);
    workspace.projects[0].archived = true;
    expect(workspaceAttention(workspace, live).map((row) => row.reason)).toEqual(['permission']);
  });
  test('does not show stale completion attention while an agent is working again', () => {
    const live = liveWorkspaceSessions({ 1: { status: 'working', attention: true } }, [{ id: 1 }], data());
    expect(workspaceAttention(data(), live)).toEqual([]);
  });
});
