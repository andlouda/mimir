import { derived, writable } from 'svelte/store';
import { agentStates } from './agentStore.js';
import { terminals } from './terminalStore.js';
import { liveWorkspaceSessions, workspaceAttention } from '../agents/workspace.js';

export const agentWorkspace = writable({ version: 1, projects: [], tasks: [], sessions: [] });
export const workspaceError = writable('');
export const liveWorkspace = derived([agentStates, terminals, agentWorkspace], ([states, terms, data]) => liveWorkspaceSessions(states, terms, data));
export const attentionItems = derived([agentWorkspace, liveWorkspace], ([data, live]) => workspaceAttention(data, live));

// Serialize reads and mutations so a late initial load cannot replace a saved
// snapshot. Failed calls leave the previous snapshot intact and remain visible.
let queue = Promise.resolve();
export function workspaceCall(method, ...args) {
  const result = queue.then(async () => {
    try {
      const fn = globalThis.window?.go?.main?.App?.[method];
      if (typeof fn !== 'function') throw new Error('Agent workspace backend is unavailable.');
      const data = JSON.parse(await fn(...args));
      if (data.version !== 1 || !Array.isArray(data.projects) || !Array.isArray(data.tasks) || !Array.isArray(data.sessions)) {
        throw new Error('Invalid agent workspace response.');
      }
      agentWorkspace.set(data);
      workspaceError.set('');
      return data;
    } catch (error) {
      workspaceError.set(String(error?.message || error));
      throw error;
    }
  });
  queue = result.catch(() => {});
  return result;
}

export const loadAgentWorkspace = () => workspaceCall('GetAgentWorkspaceJSON');
export const saveWorkspaceProject = (project) => workspaceCall('SaveAgentWorkspaceProjectJSON', JSON.stringify(project));
export const saveWorkspaceTask = (task) => workspaceCall('SaveAgentWorkspaceTaskJSON', JSON.stringify(task));
export const linkWorkspaceSession = (projectId, taskId, terminal, file) => workspaceCall('LinkAgentWorkspaceSessionJSON', projectId, taskId, terminal.id, terminal.type, file);
export const unlinkWorkspaceSession = (id) => workspaceCall('UnlinkAgentWorkspaceSessionJSON', id);
