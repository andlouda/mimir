import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  createSSHActions,
  loadSSHProfiles,
  openSSHProfilePicker,
  parseHostKeyVerifyError,
} from './sshActions.js';
import { activeTerminalId, layoutTree } from '../stores/terminalStore.js';
import {
  hostKeyVerifyState,
  showSSHProfileModal,
  sshConnecting,
  sshProfiles,
  sshSecretBackend,
} from '../stores/sshStore.js';
import { errorMessage } from '../stores/uiStore.js';

vi.mock('../../../wailsjs/go/main/App', () => ({
  AcceptSSHHostKey: vi.fn(),
  GetSSHProfiles: vi.fn(),
  GetSSHSecretBackend: vi.fn(),
  RejectSSHHostKey: vi.fn(),
  StartSSHTerminal: vi.fn(),
}));

const {
  AcceptSSHHostKey,
  GetSSHProfiles,
  GetSSHSecretBackend,
  RejectSSHHostKey,
  StartSSHTerminal,
} = await import('../../../wailsjs/go/main/App');

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  activeTerminalId.set(null);
  layoutTree.set(null);
  sshProfiles.set([]);
  sshSecretBackend.set('');
  showSSHProfileModal.set(false);
  sshConnecting.set(false);
  hostKeyVerifyState.set(null);
  errorMessage.set('');
  vi.restoreAllMocks();
});

describe('ssh actions', () => {
  test('loads profiles and secret backend', async () => {
    GetSSHProfiles.mockResolvedValue([{ id: 'prod', name: 'Prod' }]);
    GetSSHSecretBackend.mockResolvedValue('keyring');

    await loadSSHProfiles();

    expect(get(sshProfiles)).toEqual([{ id: 'prod', name: 'Prod' }]);
    expect(get(sshSecretBackend)).toBe('keyring');
  });

  test('opens profile picker and triggers profile refresh', () => {
    GetSSHProfiles.mockResolvedValue([]);
    GetSSHSecretBackend.mockResolvedValue('');

    openSSHProfilePicker();

    expect(get(showSSHProfileModal)).toBe(true);
    expect(GetSSHProfiles).toHaveBeenCalledOnce();
  });

  test('connects a profile and appends it to layout tree', async () => {
    StartSSHTerminal.mockResolvedValue(42);
    const createTerminalInstance = vi.fn().mockResolvedValue({ id: 42, type: 'ssh', name: 'SSH: Prod' });
    const persistTerminalState = vi.fn();
    const reinitializeTerminals = vi.fn();
    const { connectSSHProfile } = createSSHActions({ createTerminalInstance, persistTerminalState, reinitializeTerminals });

    await connectSSHProfile({ id: 'prod', name: 'Prod' });

    expect(createTerminalInstance).toHaveBeenCalledWith(42, 'ssh', 'SSH: Prod', false, 'prod', false, '', '', 'fresh');
    expect(get(activeTerminalId)).toBe(42);
    expect(get(layoutTree)).toEqual({ type: 'leaf', terminalId: 42 });
    expect(persistTerminalState).toHaveBeenCalledWith(expect.objectContaining({ id: 42, sshProfileId: 'prod' }));
    expect(reinitializeTerminals).toHaveBeenCalledOnce();
    expect(get(sshConnecting)).toBe(false);
  });

  test('records backend invalid ID as an error', async () => {
    StartSSHTerminal.mockResolvedValue(0);
    const { connectSSHProfile } = createSSHActions({ createTerminalInstance: vi.fn() });

    await connectSSHProfile({ id: 'prod', name: 'Prod' });

    expect(get(errorMessage)).toBe('Failed to start SSH terminal. The backend returned an invalid ID.');
    expect(get(sshConnecting)).toBe(false);
  });

  test('parses host-key verify errors into modal state', async () => {
    const profile = { id: 'prod', name: 'Prod' };
    const parsed = parseHostKeyVerifyError(
      'HOST_KEY_VERIFY|changed|example.test|SHA256:abc|ed25519|Key changed',
      profile
    );

    expect(parsed).toMatchObject({
      status: 'changed',
      host: 'example.test',
      fingerprint: 'SHA256:abc',
      keyType: 'ed25519',
      message: 'Key changed',
      profileID: 'prod',
      profile,
    });
  });

  test('accepts host key and retries profile connection', async () => {
    AcceptSSHHostKey.mockResolvedValue(undefined);
    StartSSHTerminal.mockResolvedValue(9);
    hostKeyVerifyState.set({ profileID: 'prod', profile: { id: 'prod', name: 'Prod' } });
    const { acceptHostKey } = createSSHActions({
      createTerminalInstance: vi.fn().mockResolvedValue({ id: 9 }),
      persistTerminalState: vi.fn(),
      reinitializeTerminals: vi.fn(),
    });

    await acceptHostKey();

    expect(AcceptSSHHostKey).toHaveBeenCalledWith('prod');
    expect(get(hostKeyVerifyState)).toBe(null);
    expect(StartSSHTerminal).toHaveBeenCalledWith('prod');
  });

  test('rejects host key and clears modal state', () => {
    hostKeyVerifyState.set({ profileID: 'prod' });
    const { rejectHostKey } = createSSHActions();

    rejectHostKey();

    expect(RejectSSHHostKey).toHaveBeenCalledWith('prod');
    expect(get(hostKeyVerifyState)).toBe(null);
  });
});
