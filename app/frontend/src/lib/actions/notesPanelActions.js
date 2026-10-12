import { get } from 'svelte/store';
import { notesPanelOpen, notesPanelWidth } from '../stores/uiStore.js';
import { handleResize } from './terminalActions.js';

// Drag on the notes panel's left edge resizes it; the width is remembered.
export function startNotesDrag(e) {
  e.preventDefault();
  const startX = e.clientX;
  const startWidth = get(notesPanelWidth);
  const onMove = (ev) => {
    notesPanelWidth.set(Math.max(250, Math.min(800, window.innerWidth - 560, startWidth + (startX - ev.clientX))));
  };
  const onUp = () => {
    // Listeners first: a storage failure must not leave the panel
    // following the pointer.
    window.removeEventListener('mousemove', onMove);
    window.removeEventListener('mouseup', onUp);
    try { localStorage.setItem('mimir-notes-width', String(get(notesPanelWidth))); } catch { /* storage unavailable */ }
    handleResize();
  };
  window.addEventListener('mousemove', onMove);
  window.addEventListener('mouseup', onUp);
}

export function closeNotesPanel() {
  notesPanelOpen.set(false);
  setTimeout(handleResize, 50);
}
