import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { get } from 'svelte/store';
import { agentDetectionEnabled, agentPanelTerminalId, agentStates } from '../stores/agentStore.js';
import { activeTerminalId, terminals } from '../stores/terminalStore.js';
import {
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

  test('watch polls and stops cleanly', () => {
    window.go.main.App.DetectAgentForTerminalJSON.mockResolvedValue(JSON.stringify({ detected: false }));
    startAgentWatch(1, 'bash');
    vi.advanceTimersByTime(2600);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(20100);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(2);
    stopAgentWatch(1);
    vi.advanceTimersByTime(60000);
    expect(window.go.main.App.DetectAgentForTerminalJSON).toHaveBeenCalledTimes(2);
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
    expect(window.go.main.App.GetAgentPaneTextJSON).toHaveBeenCalledWith(1, 'bash');
    expect(pane.text).toBe('screen');
  });

  test('loads transcripts with the terminal type', async () => {
    window.go.main.App.GetAgentTranscriptJSON.mockResolvedValue(JSON.stringify({ kind: 'claude', messages: [{ role: 'assistant', text: 'hi' }] }));
    const transcript = await loadAgentTranscript(2, 5);
    expect(window.go.main.App.GetAgentTranscriptJSON).toHaveBeenCalledWith(2, 'ssh', 5);
    expect(transcript.messages[0].text).toBe('hi');
  });
});
