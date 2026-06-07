import { afterEach, describe, expect, test } from 'vitest';
import { get } from 'svelte/store';
import {
  currentPage,
  errorMessage,
  historyConsentDismissed,
  historyTrackingEnabled,
  notesPanelOpen,
  showAIMenu,
  showFolderManager,
  transcriptViewerState,
} from './uiStore.js';

afterEach(() => {
  currentPage.set('terminals');
  errorMessage.set('');
  showAIMenu.set(false);
  notesPanelOpen.set(false);
  showFolderManager.set(false);
  historyTrackingEnabled.set(false);
  historyConsentDismissed.set(false);
  transcriptViewerState.set(null);
});

describe('ui store', () => {
  test('exposes default shell state', () => {
    expect(get(currentPage)).toBe('terminals');
    expect(get(errorMessage)).toBe('');
    expect(get(showAIMenu)).toBe(false);
    expect(get(notesPanelOpen)).toBe(false);
    expect(get(transcriptViewerState)).toBe(null);
  });

  test('supports mutating modal and history state', () => {
    showFolderManager.set(true);
    historyTrackingEnabled.set(true);
    historyConsentDismissed.set(true);
    transcriptViewerState.set({ resumeId: 'abc', label: 'Bash' });

    expect(get(showFolderManager)).toBe(true);
    expect(get(historyTrackingEnabled)).toBe(true);
    expect(get(historyConsentDismissed)).toBe(true);
    expect(get(transcriptViewerState)).toEqual({ resumeId: 'abc', label: 'Bash' });
  });
});
