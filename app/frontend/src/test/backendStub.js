// Installs a fake window.go.main.App for component tests: every binding is
// an async function that records its calls and returns the configured value
// (undefined otherwise), so a view can mount without the Go side.
export function installBackendStub(overrides = {}) {
  const calls = [];
  const app = new Proxy({}, {
    get(_, name) {
      if (typeof name !== 'string') return undefined;
      if (name in overrides) return overrides[name];
      return async (...args) => { calls.push([name, ...args]); return undefined; };
    },
  });
  globalThis.window.go = { main: { App: app } };
  return { calls, app };
}

export function removeBackendStub() {
  delete globalThis.window.go;
}
