import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { agentDetectionEnabled, agentPanelTerminalId, agentStates } from '../stores/agentStore.js';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
vi.mock('../../../wailsjs/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

import { EventsOn } from '../../../wailsjs/runtime';
import {
  handleAgentStateEvent,
  handleTerminalPrompt,
  handleTerminalTitle,
  loadAgentPaneText,
  loadAgentTranscript,
  noteTerminalOutput,
  openAgentPanel,
  runAgentDetection,
  startAgentWatch,
  stopAgentWatch,
} from './agentActions.js';

beforeEach(() => {
  vi.useFakeTimers();
  globalThis.window = {
    go: { main: { App: { DetectAgentForTerminalJSON: vi.fn(), GetAgentTranscriptJSON: vi.fn(), GetAgentPaneTextJSON: vi.fn() } } },
  };
  terminals.set([{ id: 1, type: 'bash' }, { id: 2, type: 'ssh' }]);
  activeTerminalId.set(1);
  agentStates.set({});
  agentPanelTerminalId.set(null);
  agentDetectionEnabled.set(true);
});

afterEach(() => {
  stopAgentWatch(1);
  stopAgentWatch(2);
  vi.useRealTimers();
  vi.clearAllMocks();
  delete globalThis.window;
});

describe('agent detection', () => {
  test('stores a detection result and clears it when the agent is gone', async () => {
    window.go.main.App.DetectAgentForTerminalJSON.mockResolvedValueOnce(JSON.stringify({
      detected: true, kind: 'claude', label: 'Claude Code', pid: 4300, cwd: '/home/u/proj', source: 'local', transcripts: true,
    }));
    await runAgentDetection(1);
    expect(get(agentStates)[1]).toMatchObject({ kind: 'claude', label: 'Claude Code', cwd: '/home/u/proj', status: 'unknown' });
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledWith(1, 'bash');

    openAgentPanel(1);
    window.go.main.App.DetectAgentForTerminalJSON.mockResolvedValueOnce(JSON.stringify({ detected: false }));
    await runAgentDetection(1);
    expect(get(agentStates)[1]).toBeUndefined();
    expect(get(agentPanelTerminalId)).toBeNull();
  });

  test('title changes drive status and flag attention for inactive panes', () => {
    agentStates.set({ 2: { kind: 'claude', label: 'Claude Code', status: 'unknown', attention: false } });
    handleTerminalTitle(2, '◐ Bash curl example');
    expect(get(agentStates)[2]).toMatchObject({ status: 'working', subject: 'Bash curl example', attention: false });

    handleTerminalTitle(2, '✳ Bash curl example');
    expect(get(agentStates)[2]).toMatchObject({ status: 'idle', attention: true });

    // Activating the pane clears the marker.
    activeTerminalId.set(2);
    expect(get(agentStates)[2].attention).toBe(false);

    // The active pane finishing does not raise attention.
    handleTerminalTitle(2, '◑ next');
    handleTerminalTitle(2, '✳ next');
    expect(get(agentStates)[2].attention).toBe(false);
  });

  test('unchanged detections and title ticks do not rewrite the store', async () => {
    const payload = JSON.stringify({ detected: true, kind: 'claude', label: 'Claude Code', pid: 1, cwd: '/p', source: 'local', transcripts: true });
    window.go.main.App.DetectAgentForTerminalJSON.mockResolvedValue(payload);
    await runAgentDetection(1);
    const before = get(agentStates);
    await runAgentDetection(1);
    expect(get(agentStates)).toBe(before);

    handleTerminalTitle(1, '◐ Bash');
    const afterTitle = get(agentStates);
    handleTerminalTitle(1, '◑ Bash');
    expect(get(agentStates)).toBe(afterTitle);
  });

  test('agent-like title without known agent triggers an immediate probe', () => {
    window.go.main.App.DetectAgentForTerminalJSON.mockResolvedValue(JSON.stringify({ detected: false }));
    handleTerminalTitle(1, '✳ Claude Code');
    vi.advanceTimersByTime(1);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(1);
  });

  test('output banners schedule a debounced probe only when nothing is known', () => {
    window.go.main.App.DetectAgentForTerminalJSON.mockResolvedValue(JSON.stringify({ detected: false }));
    noteTerminalOutput(1, 'Welcome to OpenAI Codex');
    noteTerminalOutput(1, 'Welcome to OpenAI Codex');
    vi.advanceTimersByTime(2000);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(1);

    agentStates.set({ 1: { kind: 'codex' } });
    noteTerminalOutput(1, 'Welcome to OpenAI Codex');
    vi.advanceTimersByTime(2000);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(1);
  });

  test('idle terminals are probed once at start-up and never polled', async () => {
    window.go.main.App.DetectAgentForTerminalJSON.mockResolvedValue(JSON.stringify({ detected: false }));
    startAgentWatch(1, 'bash');
    vi.advanceTimersByTime(2600);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(10 * 60 * 1000);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(1);
    // A prompt without a known agent is ignored too.
    handleTerminalPrompt(1);
    vi.advanceTimersByTime(5000);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(1);
  });

  test('a known agent gets a slow liveness check and a prompt re-check', async () => {
    const detected = JSON.stringify({ detected: true, kind: 'codex', label: 'Codex', pid: 7, cwd: '/p', source: 'local', transcripts: true });
    window.go.main.App.DetectAgentForTerminalJSON.mockResolvedValue(detected);
    startAgentWatch(1, 'bash');
    await runAgentDetection(1);
    expect(get(agentStates)[1].kind).toBe('codex');
    vi.advanceTimersByTime(2600); // the one start-up probe
    const calls = window.go.main.App.DetectAgentForTerminalJSON.mock.calls.length;

    vi.advanceTimersByTime(3 * 60 * 1000 + 10);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(calls + 1);

    // Prompt beacon → agent probably exited → re-check, which now says gone.
    window.go.main.App.DetectAgentForTerminalJSON.mockResolvedValue(JSON.stringify({ detected: false }));
    handleTerminalPrompt(1);
    vi.advanceTimersByTime(2000);
    await vi.runOnlyPendingTimersAsync();
    expect(get(agentStates)[1]).toBeUndefined();
    const after = window.go.main.App.DetectAgentForTerminalJSON.mock.calls.length;
    vi.advanceTimersByTime(10 * 60 * 1000);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(after);
    stopAgentWatch(1);
  });

  test('disabling detection drops state and skips probes', () => {
    agentStates.set({ 1: { kind: 'claude' } });
    agentDetectionEnabled.set(false);
    expect(get(agentStates)).toEqual({});
    handleTerminalTitle(1, '◐ x');
    vi.advanceTimersByTime(5000);
    expect(window.go.main.App.DetectAgentForTerminalJSON).not.toHaveBeenCalled();
  });

  test('loads the tmux pane text with the terminal type', async () => {
    window.go.main.App.GetAgentPaneTextJSON.mockResolvedValue(JSON.stringify({ text: 'screen', width: 80, lines: 1 }));
    const pane = await loadAgentPaneText(1);
    expect(window.go.main.App.GetAgentPaneTextJSON).toHaveBeenCalledWith(1, 'bash', false);
    await loadAgentPaneText(1, { full: true });
    expect(window.go.main.App.GetAgentPaneTextJSON).toHaveBeenLastCalledWith(1, 'bash', true);
    expect(pane.text).toBe('screen');
  });

  test('loads transcripts with the terminal type', async () => {
    window.go.main.App.GetAgentTranscriptJSON.mockResolvedValue(JSON.stringify({ kind: 'claude', messages: [{ role: 'assistant', text: 'hi' }] }));
    const transcript = await loadAgentTranscript(2, 5);
    expect(window.go.main.App.GetAgentTranscriptJSON).toHaveBeenCalledWith(2, 'ssh', 5);
    expect(transcript.messages[0].text).toBe('hi');
  });

  test('session-file state events drive status and silence title parsing', async () => {
    window.go.main.App.DetectAgentForTerminalJSON.mockResolvedValue(JSON.stringify({
      detected: true, kind: 'claude', label: 'Claude Code', pid: 1, cwd: '/p', source: 'local', transcripts: true, tmux: true,
    }));
    startAgentWatch(2, 'bash');
    await runAgentDetection(2);
    expect(EventsOn).toHaveBeenCalledWith('agent-state-2', expect.any(Function));

    activeTerminalId.set(1);
    handleAgentStateEvent(2, JSON.stringify({ state: 'working', lastText: 'Looking at the tests.\nmore', lastAt: 't1' }));
    expect(get(agentStates)[2]).toMatchObject({ status: 'working', subject: 'Looking at the tests.', fileState: true, attention: false });

    handleAgentStateEvent(2, JSON.stringify({ state: 'idle', lastText: 'Done. Shall I commit?', lastAt: 't2', sessionFile: '/s.jsonl' }));
    expect(get(agentStates)[2]).toMatchObject({ status: 'idle', subject: 'Done. Shall I commit?', attention: true, sessionFile: '/s.jsonl' });

    // A title tick must not override the file-derived state any more.
    handleTerminalTitle(2, '◐ Bash something');
    expect(get(agentStates)[2].status).toBe('idle');

    // Garbage payloads are ignored.
    handleAgentStateEvent(2, 'not json');
    expect(get(agentStates)[2].status).toBe('idle');
    stopAgentWatch(2);
  });
});
