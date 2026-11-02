import { afterEach, describe, expect, it } from 'vitest';
import { resetWindowsBuildNumber, windowsPtyOptions } from './windowsPty.js';

function stubBackend(methods) {
  globalThis.window = { go: { main: { App: methods } } };
}

describe('windowsPtyOptions', () => {
  afterEach(() => {
    delete globalThis.window;
    resetWindowsBuildNumber();
  });

  it('is unset off Windows', async () => {
    stubBackend({ GetWindowsBuildNumber: async () => 22631 });
    expect(await windowsPtyOptions('bash', { windows: false })).toBeUndefined();
  });

  it('is unset for SSH panes, whose PTY is remote', async () => {
    stubBackend({ GetWindowsBuildNumber: async () => 22631 });
    expect(await windowsPtyOptions('ssh', { windows: true })).toBeUndefined();
  });

  it('reports ConPTY with the build number for local Windows panes', async () => {
    let asked = 0;
    stubBackend({ GetWindowsBuildNumber: async () => { asked += 1; return 22631; } });
    expect(await windowsPtyOptions('powershell', { windows: true })).toEqual({ backend: 'conpty', buildNumber: 22631 });
    expect(await windowsPtyOptions('wsl', { windows: true })).toEqual({ backend: 'conpty', buildNumber: 22631 });
    expect(asked).toBe(1); // asked once per page load
  });

  it('reports ConPTY without a build number when the backend has none', async () => {
    stubBackend({ GetWindowsBuildNumber: async () => 0 });
    expect(await windowsPtyOptions('cmd', { windows: true })).toEqual({ backend: 'conpty' });
  });

  it('survives a failing or missing backend method', async () => {
    stubBackend({ GetWindowsBuildNumber: async () => { throw new Error('old backend'); } });
    expect(await windowsPtyOptions('cmd', { windows: true })).toEqual({ backend: 'conpty' });
    resetWindowsBuildNumber();
    delete globalThis.window;
    expect(await windowsPtyOptions('cmd', { windows: true })).toEqual({ backend: 'conpty' });
  });
});
