import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { SaveCurrentSession } from '../../../wailsjs/go/main/App';
import { getTranscriptExcerpt } from '../transcript/transcriptApi.js';
import { sanitizeTranscriptPreview } from '../util.js';
import {
  clearSessionSaveTimer,
  loadTranscriptExcerpt,
  persistTerminalState,
  scheduleSessionSave,
} from './sessionActions.js';

vi.mock('../../../wailsjs/go/main/App', () => ({
  SaveCurrentSession: vi.fn(() => Promise.resolve()),
}));

vi.mock('../transcript/transcriptApi.js', () => ({
  getTranscriptExcerpt: vi.fn(),
}));

vi.mock('../util.js', () => ({
  sanitizeTranscriptPreview: vi.fn((raw) => `clean:${raw}`),
}));

let updateTerminalState;

beforeEach(() => {
  vi.useFakeTimers();
  updateTerminalState = vi.fn();
  global.window = { go: { main: { App: { UpdateTerminalState: updateTerminalState } } } };
});

afterEach(() => {
  clearSessionSaveTimer();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.clearAllMocks();
  delete global.window;
});

describe('scheduleSessionSave', () => {
  test('debounces and saves once after the delay', () => {
    scheduleSessionSave(250);
    scheduleSessionSave(250);
    scheduleSessionSave(250);

    expect(SaveCurrentSession).not.toHaveBeenCalled();
    vi.advanceTimersByTime(250);
    expect(SaveCurrentSession).toHaveBeenCalledTimes(1);
  });

  test('clearSessionSaveTimer cancels a pending save', () => {
    scheduleSessionSave(250);
    clearSessionSaveTimer();
    vi.advanceTimersByTime(500);
    expect(SaveCurrentSession).not.toHaveBeenCalled();
  });
});

describe('persistTerminalState', () => {
  test('forwards normalized fields to the backend and schedules a save', () => {
    persistTerminalState({
      id: 7,
      type: 'bash',
      name: 'Shell',
      minimized: false,
    });

    expect(updateTerminalState).toHaveBeenCalledWith(7, 'bash', 'Shell', false, '', '', '', 'fresh', '');
    vi.advanceTimersByTime(250);
    expect(SaveCurrentSession).toHaveBeenCalledTimes(1);
  });

  test('applies overrides over the base terminal', () => {
    persistTerminalState(
      { id: 3, type: 'ssh', name: 'Box', minimized: false, sshProfileId: 'p1' },
      { minimized: true },
    );

    expect(updateTerminalState).toHaveBeenCalledWith(3, 'ssh', 'Box', true, 'p1', '', '', 'fresh', '');
  });
});

describe('loadTranscriptExcerpt', () => {
  test('returns empty string when no resumeId given', async () => {
    expect(await loadTranscriptExcerpt('')).toBe('');
    expect(getTranscriptExcerpt).not.toHaveBeenCalled();
  });

  test('sanitizes the excerpt from the backend', async () => {
    getTranscriptExcerpt.mockResolvedValue('raw-data');

    const result = await loadTranscriptExcerpt('resume-1', 4000);

    expect(getTranscriptExcerpt).toHaveBeenCalledWith('resume-1', 4000);
    expect(sanitizeTranscriptPreview).toHaveBeenCalledWith('raw-data');
    expect(result).toBe('clean:raw-data');
  });

  test('returns empty string on backend failure', async () => {
    getTranscriptExcerpt.mockRejectedValue(new Error('boom'));
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    expect(await loadTranscriptExcerpt('resume-2')).toBe('');
    errorSpy.mockRestore();
  });
});
