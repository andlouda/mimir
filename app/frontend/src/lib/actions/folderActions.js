import { get } from 'svelte/store';
import { DeleteTerminalFolder, GetTerminalFolders, SaveTerminalFolder, UpdateTerminalFolder } from '../../../wailsjs/go/main/App';
import { customFolders, newFolderName, terminalSessionFoldersOpen } from '../stores/sessionStore.js';
import { terminals } from '../stores/terminalStore.js';
import { errorMessage } from '../stores/uiStore.js';

export function toggleTerminalFolder(groupID) {
  const current = get(terminalSessionFoldersOpen);
  terminalSessionFoldersOpen.set({
    ...current,
    [groupID]: !(current[groupID] ?? true),
  });
}

export async function loadCustomFolders() {
  try {
    customFolders.set(await GetTerminalFolders());
  } catch (error) {
    console.error('Failed to load terminal folders:', error);
  }
}

export async function createFolder() {
  const name = get(newFolderName).trim();
  if (!name) return;
  try {
    const folders = get(customFolders);
    customFolders.set(await SaveTerminalFolder(JSON.stringify({ name, position: folders.length + 1 })));
    newFolderName.set('');
  } catch (error) {
    errorMessage.set(`Failed to create folder: ${error.message || error}`);
  }
}

export async function renameFolder(folder, newName) {
  const trimmed = newName.trim();
  if (!trimmed || trimmed === folder.name) return;
  try {
    customFolders.set(await UpdateTerminalFolder(JSON.stringify({ ...folder, name: trimmed })));
  } catch (error) {
    errorMessage.set(`Failed to rename folder: ${error.message || error}`);
  }
}

export async function deleteFolder(folderId, persistTerminalState) {
  try {
    customFolders.set(await DeleteTerminalFolder(folderId));
    const currentTerminals = get(terminals);
    const affected = currentTerminals.filter((terminal) => terminal.folderId === folderId);
    terminals.set(currentTerminals.map((terminal) => (terminal.folderId === folderId ? { ...terminal, folderId: '' } : terminal)));
    for (const terminal of affected) {
      persistTerminalState?.({ ...terminal, folderId: '' });
    }
  } catch (error) {
    errorMessage.set(`Failed to delete folder: ${error.message || error}`);
  }
}

export function assignTerminalToFolder(terminalId, folderId, persistTerminalState) {
  terminals.update((currentTerminals) => currentTerminals.map((terminal) => (terminal.id === terminalId ? { ...terminal, folderId } : terminal)));
  const term = get(terminals).find((terminal) => terminal.id === terminalId);
  if (term) persistTerminalState?.(term);
}
