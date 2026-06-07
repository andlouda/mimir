import { describe, expect, test } from 'vitest';
import { markReconnectFailed, markReconnectStarted, markReconnectSucceeded } from './reconnectLifecycle.js';

const terminals = [
  { id: 1, type: 'bash', disconnected: false, reconnecting: false },
  { id: 2, type: 'ssh', disconnected: true, reconnecting: false },
];

describe('terminal reconnect lifecycle', () => {
  test('marks only the target terminal as reconnecting', () => {
    const next = markReconnectStarted(terminals, 2);
    expect(next[0]).toEqual(terminals[0]);
    expect(next[1]).toMatchObject({ id: 2, disconnected: true, reconnecting: true });
  });

  test('clears disconnected and reconnecting on success and writes status text', () => {
    const writes = [];
    const next = markReconnectSucceeded(
      [{ ...terminals[1], reconnecting: true }],
      2,
      (term, data) => writes.push({ id: term.id, data }),
    );

    expect(next[0]).toMatchObject({ id: 2, disconnected: false, reconnecting: false });
    expect(writes).toEqual([{ id: 2, data: '\r\n\x1b[32m--- Reconnected ---\x1b[0m\r\n' }]);
  });

  test('keeps disconnected state on failure and writes the error', () => {
    const writes = [];
    const next = markReconnectFailed(
      [{ ...terminals[1], reconnecting: true }],
      2,
      new Error('network down'),
      (term, data) => writes.push({ id: term.id, data }),
    );

    expect(next[0]).toMatchObject({ id: 2, disconnected: true, reconnecting: false });
    expect(writes[0].id).toBe(2);
    expect(writes[0].data).toContain('Reconnect failed: network down');
  });
});
