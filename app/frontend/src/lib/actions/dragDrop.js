import { get } from 'svelte/store';
import { moveLeaf, swapLeaves } from '../terminals/layoutTree';
import { layoutTree, draggingTerminalId } from '../stores/terminalStore.js';

export function createDragDropHandlers({ reinitializeTerminals, documentRef = () => document } = {}) {
  function handleDragStart(event, id) {
    draggingTerminalId.set(id);
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', String(id));
    setTimeout(() => {
      event.currentTarget.classList.add('dragging');
    }, 0);
  }

  function handleDragOver(event) {
    // Allow the drop; the precise zone is computed in the pane overlay.
    event.preventDefault();
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = 'move';
    }
  }

  function handleDragLeave() {}

  // handleDrop relocates the dragged terminal next to the target. With a zone
  // ('left'|'right'|'top'|'bottom') the split is reoriented accordingly; without
  // one it falls back to swapping the two panes.
  function handleDrop(event, targetId, zone) {
    event.preventDefault();

    const stored = get(draggingTerminalId);
    const transferred = parseInt(event.dataTransfer?.getData('text/plain'), 10);
    const draggedId = Number.isInteger(transferred) ? transferred : stored;
    draggingTerminalId.set(null);

    if (!draggedId || draggedId === targetId) return;

    const tree = get(layoutTree);
    const next = zone ? moveLeaf(tree, draggedId, targetId, zone) : swapLeaves(tree, draggedId, targetId);
    layoutTree.set(next);
    if (typeof reinitializeTerminals === 'function') {
      reinitializeTerminals();
    }
  }

  function handleDragEnd(event) {
    event.target.classList.remove('dragging');
    draggingTerminalId.set(null);
    documentRef().querySelectorAll('.drag-over-top, .drag-over-bottom').forEach((el) => {
      el.classList.remove('drag-over-top', 'drag-over-bottom');
    });
  }

  return {
    handleDragStart,
    handleDragOver,
    handleDragLeave,
    handleDrop,
    handleDragEnd,
  };
}
