import { afterEach, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { agentWorkspace, workspaceError, loadAgentWorkspace, saveWorkspaceProject } from './agentWorkspaceStore.js';

afterEach(() => { delete globalThis.window; });

test('failed persistence keeps the previous snapshot and exposes the failure', async () => {
  const original = { version: 1, projects: [{ id: 'p' }], tasks: [], sessions: [] };
  agentWorkspace.set(original);
  globalThis.window = { go: { main: { App: { SaveAgentWorkspaceProjectJSON: vi.fn().mockRejectedValue(new Error('disk full')) } } } };
  await expect(saveWorkspaceProject({ name: 'new' })).rejects.toThrow('disk full');
  expect(get(agentWorkspace)).toBe(original);
  expect(get(workspaceError)).toBe('disk full');
});

test('an initial load cannot overtake a queued save', async () => {
  let resolveLoad;
  const empty = { version: 1, projects: [], tasks: [], sessions: [] };
  const saved = { ...empty, projects: [{ id: 'p', name: 'Mimir' }] };
  const save = vi.fn().mockResolvedValue(JSON.stringify(saved));
  globalThis.window = { go: { main: { App: {
    GetAgentWorkspaceJSON: () => new Promise((resolve) => { resolveLoad = resolve; }),
    SaveAgentWorkspaceProjectJSON: save,
  } } } };
  const loading = loadAgentWorkspace();
  const saving = saveWorkspaceProject({ name: 'Mimir' });
  await vi.waitFor(() => expect(resolveLoad).toBeTypeOf('function'));
  expect(save).not.toHaveBeenCalled();
  resolveLoad(JSON.stringify(empty));
  await Promise.all([loading, saving]);
  expect(get(agentWorkspace)).toEqual(saved);
  expect(get(workspaceError)).toBe('');
});
