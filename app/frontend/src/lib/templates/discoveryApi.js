// Shared entry point for discovery calls (template prompts, workflow builder,
// function catalog).
//
// Prefers the terminal-aware backend, which runs discovery where the terminal
// lives: on the remote host for SSH sessions, in the tmux pane's current
// directory for local sessions. Falls back to the legacy app-cwd variant when
// no terminal is available or the backend is older.
export function runDiscovery(terminalId, discoveryTool, terminalType, variablesJSON) {
  const api = window['go']['main']['App'];
  const id = Number(terminalId);
  if (typeof api['RunDiscoveryForTerminalJSON'] === 'function' && Number.isFinite(id) && id > 0) {
    return api['RunDiscoveryForTerminalJSON'](id, discoveryTool, terminalType, variablesJSON);
  }
  return api['RunDiscoveryJSON'](discoveryTool, terminalType, variablesJSON);
}
