import { describe, expect, test, vi } from 'vitest';
vi.mock('../../../wailsjs/runtime', () => ({ EventsOn: () => () => {}, ClipboardSetText: async () => {} }));
import { lastUsedText, resumeCommand } from './agentActions.js';

describe('offline sessions', () => {
  test('last used is short and relative, then a date', () => {
    const now = Date.parse('2026-10-07T12:00:00Z');
    expect(lastUsedText('2026-10-07T11:59:40Z', now)).toBe('now');
    expect(lastUsedText('2026-10-07T11:45:00Z', now)).toBe('15 min');
    expect(lastUsedText('2026-10-07T09:00:00Z', now)).toBe('3 h');
    expect(lastUsedText('2026-10-02T12:00:00Z', now)).toBe('5 d');
    expect(lastUsedText('2026-08-01T12:00:00Z', now)).not.toMatch(/ d$/);
    expect(lastUsedText('', now)).toBe('');
  });
  test('resume command uses the session id', () => {
    expect(resumeCommand({ id: 'aaaa-1' })).toBe('claude --resume aaaa-1');
  });
});
