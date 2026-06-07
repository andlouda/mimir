import { get } from 'svelte/store';
import {
  AcceptSSHHostKey,
  GetSSHProfiles,
  GetSSHSecretBackend,
  RejectSSHHostKey,
  StartSSHTerminal,
} from '../../../wailsjs/go/main/App';
import { activeTerminalId, layoutTree } from '../stores/terminalStore.js';
import {
  hostKeyVerifyState,
  showSSHProfileModal,
  sshConnecting,
  sshProfiles,
  sshSecretBackend,
} from '../stores/sshStore.js';
import { errorMessage } from '../stores/uiStore.js';

export async function loadSSHProfiles() {
  try {
    sshProfiles.set(await GetSSHProfiles());
    sshSecretBackend.set(await GetSSHSecretBackend());
  } catch (error) {
    console.error('Failed to load SSH profiles:', error);
  }
}

export function openSSHProfilePicker() {
  loadSSHProfiles();
  showSSHProfileModal.set(true);
}

function appendTerminalLeaf(id) {
  const newLeaf = { type: 'leaf', terminalId: id };
  const currentTree = get(layoutTree);
  if (currentTree === null) {
    layoutTree.set(newLeaf);
    return;
  }

  layoutTree.set({
    type: 'split',
    direction: 'horizontal',
    ratio: 0.5,
    children: [currentTree, newLeaf],
  });
}

export function parseHostKeyVerifyError(errorMessageText, profile) {
  if (!String(errorMessageText || '').includes('HOST_KEY_VERIFY|')) return null;

  const payload = String(errorMessageText).substring(String(errorMessageText).indexOf('HOST_KEY_VERIFY|') + 16);
  const parts = payload.split('|');
  return {
    status: parts[0] || 'unknown',
    host: parts[1] || '',
    fingerprint: parts[2] || '',
    keyType: parts[3] || '',
    message: parts[4] || '',
    profileID: profile.id,
    profile,
  };
}

export function createSSHActions({ createTerminalInstance, persistTerminalState, reinitializeTerminals } = {}) {
  async function connectSSHProfile(profile) {
    try {
      sshConnecting.set(true);
      const id = await StartSSHTerminal(profile.id);
      if (!id) {
        errorMessage.set('Failed to start SSH terminal. The backend returned an invalid ID.');
        sshConnecting.set(false);
        return;
      }

      showSSHProfileModal.set(false);
      sshConnecting.set(false);

      appendTerminalLeaf(id);

      const newTerminal = await createTerminalInstance(id, 'ssh', `SSH: ${profile.name}`, false, profile.id, false, '', '', 'fresh');
      activeTerminalId.set(id);
      persistTerminalState?.({ ...newTerminal, type: 'ssh', minimized: false, sshProfileId: profile.id });
      await reinitializeTerminals?.();
    } catch (error) {
      sshConnecting.set(false);
      const errMsg = error.message || String(error);
      const hostKeyState = parseHostKeyVerifyError(errMsg, profile);
      if (hostKeyState) {
        hostKeyVerifyState.set(hostKeyState);
        return;
      }
      errorMessage.set(`SSH connection failed: ${errMsg}`);
    }
  }

  async function acceptHostKey() {
    const state = get(hostKeyVerifyState);
    if (!state) return;
    const { profileID, profile } = state;
    try {
      await AcceptSSHHostKey(profileID);
      hostKeyVerifyState.set(null);
      await connectSSHProfile(profile);
    } catch (error) {
      errorMessage.set(`Failed to accept host key: ${error.message || error}`);
      hostKeyVerifyState.set(null);
    }
  }

  function rejectHostKey() {
    const state = get(hostKeyVerifyState);
    if (!state) return;
    RejectSSHHostKey(state.profileID);
    hostKeyVerifyState.set(null);
  }

  return {
    connectSSHProfile,
    acceptHostKey,
    rejectHostKey,
  };
}
