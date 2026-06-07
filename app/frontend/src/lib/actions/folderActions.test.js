import { afterEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { DeleteTerminalFolder, GetTerminalFolders, SaveTerminalFolder, UpdateTerminalFolder } from '../../../wailsjs/go/main/App';
import { customFolders, newFolderName, terminalSessionFoldersOpen } from '../stores/sessionStore.js';
import { terminals } from '../stores/terminalStore.js';
import { errorMessage } from '../stores/uiStore.js';
import {
  assignTerminalToFolder,
  createFolder,
  deleteFolder,
  loadCustomFolders,
  renameFolder,
  toggleTerminalFolder,
} from './folderActions.js';

vi.mock('../../../wailsjs/go/main/App', () => ({
  DeleteTerminalFolder: vi.fn(),
  GetTerminalFolders: vi.fn(),
  SaveTerminalFolder: vi.fn(),
  UpdateTerminalFolder: vi.fn(),
}));

afterEach(() => {
  customFolders.set([]);
  newFolderName.set('');
  terminalSessionFoldersOpen.set({ local: true, ssh: true, windows: true, other: true });
  terminals.set([]);
  errorMessage.set('');
  vi.clearAllMocks();
});

describe('folder actions', () => {
  test('loads folders from the backend', async () => {
    GetTerminalFolders.mockResolvedValue([{ id: 'ops', name: 'Ops', position: 1 }]);

    await loadCustomFolders();

    expect(get(customFolders)).toEqual([{ id: 'ops', name: 'Ops', position: 1 }]);
  });

  test('creates folders and clears the input', async () => {
    customFolders.set([{ id: 'old', name: 'Old', position: 1 }]);
    newFolderName.set('New');
    SaveTerminalFolder.mockResolvedValue([{ id: 'new', name: 'New', position: 2 }]);

    await createFolder();

    expect(SaveTerminalFolder).toHaveBeenCalledWith(JSON.stringify({ name: 'New', position: 2 }));
    expect(get(customFolders)).toEqual([{ id: 'new', name: 'New', position: 2 }]);
    expect(get(newFolderName)).toBe('');
  });

  test('renames a folder through the backend', async () => {
    UpdateTerminalFolder.mockResolvedValue([{ id: 'ops', name: 'Platform', position: 1 }]);

    await renameFolder({ id: 'ops', name: 'Ops', position: 1 }, 'Platform');

    expect(UpdateTerminalFolder).toHaveBeenCalledWith(JSON.stringify({ id: 'ops', name: 'Platform', position: 1 }));
    expect(get(customFolders)[0].name).toBe('Platform');
  });

  test('deletes a folder and persists affected terminals', async () => {
    terminals.set([{ id: 1, folderId: 'ops' }, { id: 2, folderId: '' }]);
    DeleteTerminalFolder.mockResolvedValue([]);
    const persistTerminalState = vi.fn();

    await deleteFolder('ops', persistTerminalState);

    expect(get(terminals).find((terminal) => terminal.id === 1).folderId).toBe('');
    expect(persistTerminalState).toHaveBeenCalledWith({ id: 1, folderId: '' });
  });

  test('assigns a terminal to a folder', () => {
    terminals.set([{ id: 1, folderId: '' }]);
    const persistTerminalState = vi.fn();

    assignTerminalToFolder(1, 'ops', persistTerminalState);

    expect(get(terminals)[0].folderId).toBe('ops');
    expect(persistTerminalState).toHaveBeenCalledWith({ id: 1, folderId: 'ops' });
  });

  test('toggles folder open state', () => {
    toggleTerminalFolder('local');

    expect(get(terminalSessionFoldersOpen).local).toBe(false);
  });
});
