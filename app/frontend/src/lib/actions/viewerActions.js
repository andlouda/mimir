import { get } from 'svelte/store';
import { terminals } from '../stores/terminalStore.js';
import { dotEnvViewerState, transcriptViewerState } from '../stores/uiStore.js';

export function openTranscriptViewer(id) {
  const term = get(terminals).find((t) => t.id === id);
  transcriptViewerState.set({
    resumeId: term?.resumeId || '',
    label: term?.name || '',
  });
}

export function openDotEnvViewer(detail) {
  const term = get(terminals).find((t) => t.id === detail.id);
  dotEnvViewerState.set({
    terminalId: detail.id,
    terminalType: detail.type || term?.type || '',
    label: term?.name || '',
  });
}
