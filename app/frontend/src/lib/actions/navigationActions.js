import { get } from 'svelte/store';
import { GetAggStatus, GetSSHTerminalLabel, IsAggInstalled, ListRecordings } from '../backend.js';
import { aggAvailable, aggStatus, recordingList } from '../stores/sessionStore.js';
import { fileBrowserRemoteLabel, fileBrowserRemoteTerminalId } from '../stores/sshStore.js';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
import { currentPage, errorMessage } from '../stores/uiStore.js';
import { focusTerminal } from './keyboardShortcuts.js';
import { loadTemplatesFromBackend } from './templateActions.js';
import { reinitializeTerminals } from './terminalActions.js';

// openPage switches the main view and loads what the page needs on entry.
export async function openPage(page) {
  currentPage.set(page);

  if (page === 'terminals') {
    await reinitializeTerminals();
    focusTerminal(get(activeTerminalId));
    return;
  }

  if (page === 'recordings' || page === 'settings') {
    aggAvailable.set(await IsAggInstalled().catch(() => false));
    aggStatus.set(await GetAggStatus().catch(() => 'missing'));
  }

  if (page === 'recordings') {
    try {
      recordingList.set(await ListRecordings());
    } catch (e) {
      console.error('Failed to load recordings:', e);
    }
  }

  if (page === 'templateManager') {
    try {
      await loadTemplatesFromBackend();
    } catch (error) {
      errorMessage.set(`Failed to load templates: ${error.message || error}`);
    }
  }

  if (page === 'fileBrowser') {
    const activeTerm = get(terminals).find((t) => t.id === get(activeTerminalId));
    if (activeTerm && activeTerm.type === 'ssh' && !activeTerm.disconnected) {
      try {
        const label = await GetSSHTerminalLabel(activeTerm.id);
        if (label) {
          fileBrowserRemoteTerminalId.set(activeTerm.id);
          fileBrowserRemoteLabel.set(label);
          return;
        }
      } catch (e) {
        // Fall through to local mode
      }
    }
    fileBrowserRemoteTerminalId.set(0);
    fileBrowserRemoteLabel.set('');
  }
}
