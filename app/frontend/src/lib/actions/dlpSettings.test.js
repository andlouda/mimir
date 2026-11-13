import { describe, expect, test, vi } from 'vitest';
import { DLP_DEFAULTS, loadDLPSettings, saveDLPSettings } from './dlpSettings.js';

describe('local masking model settings', () => {
  test('loads what the backend has and falls back to "off" without one', async () => {
    const load = vi.fn(async () => JSON.stringify({ localModelEnabled: true, baseUrl: 'http://localhost:11434/api/chat', model: 'tiny' }));
    expect(await loadDLPSettings({ load })).toEqual({ localModelEnabled: true, baseUrl: 'http://localhost:11434/api/chat', model: 'tiny' });
    expect(await loadDLPSettings({ load: undefined })).toEqual(DLP_DEFAULTS);
    expect(await loadDLPSettings({ load: async () => '' })).toEqual(DLP_DEFAULTS);
  });

  test('saves a patch on top of the current settings', async () => {
    const save = vi.fn(async (json) => json);
    const current = { localModelEnabled: false, baseUrl: 'http://localhost:11434/api/chat', model: 'tiny' };
    const stored = await saveDLPSettings(current, { localModelEnabled: true }, { save });
    expect(JSON.parse(save.mock.calls[0][0])).toEqual({ ...current, localModelEnabled: true });
    expect(stored.localModelEnabled).toBe(true);
  });

  test('passes the backend refusal on and refuses without a backend', async () => {
    const save = vi.fn(async () => { throw new Error('local model host "8.8.8.8" is not on this machine or a private network'); });
    await expect(saveDLPSettings(DLP_DEFAULTS, { baseUrl: 'https://8.8.8.8/x' }, { save })).rejects.toThrow('private network');
    await expect(saveDLPSettings(DLP_DEFAULTS, {}, { save: undefined })).rejects.toThrow('no local masking model');
  });
});
