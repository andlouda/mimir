export function markReconnectStarted(terminals, id) {
  return terminals.map((term) => (term.id === id ? { ...term, reconnecting: true } : term));
}

export function markReconnectSucceeded(terminals, id, writeTerminal) {
  return terminals.map((term) => {
    if (term.id !== id) return term;
    writeTerminal?.(term, '\r\n\x1b[32m--- Reconnected ---\x1b[0m\r\n');
    return { ...term, disconnected: false, reconnecting: false };
  });
}

export function markReconnectFailed(terminals, id, error, writeTerminal) {
  const message = error?.message || error || 'Unknown error';
  return terminals.map((term) => {
    if (term.id !== id) return term;
    writeTerminal?.(term, `\r\n\x1b[31mReconnect failed: ${message}\x1b[0m\r\n`);
    return { ...term, reconnecting: false };
  });
}
