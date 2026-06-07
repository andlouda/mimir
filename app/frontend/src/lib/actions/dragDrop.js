import { get } from 'svelte/store';
import { swapLeaves } from '../terminals/layoutTree';
import { layoutTree } from '../stores/terminalStore.js';

export function createDragDropHandlers({ reinitializeTerminals, documentRef = () => document } = {}) {
  let draggedTerminalId = null;

  function handleDragStart(event, id) {
    draggedTerminalId = id;
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', id);
    setTimeout(() => {
      event.currentTarget.classList.add('dragging');
    }, 0);
  }

  function handleDragOver(event, id) {
    event.preventDefault();
    const targetWrapper = event.currentTarget;
    if (targetWrapper.classList.contains('terminal-header') && draggedTerminalId !== id) {
      const bounding = targetWrapper.getBoundingClientRect();
      const offset = event.clientY - bounding.top;
      if (offset > bounding.height / 2) {
        targetWrapper.classList.remove('drag-over-top');
        targetWrapper.classList.add('drag-over-bottom');
      } else {
        targetWrapper.classList.remove('drag-over-bottom');
        targetWrapper.classList.add('drag-over-top');
      }
    }
  }

  function handleDragLeave(event) {
    event.currentTarget.classList.remove('drag-over-top', 'drag-over-bottom');
  }

  function handleDrop(event, targetId) {
    event.preventDefault();
    event.currentTarget.classList.remove('drag-over-top', 'drag-over-bottom');

    const draggedId = parseInt(event.dataTransfer.getData('text/plain'), 10);
    if (draggedId && draggedId !== targetId) {
      layoutTree.set(swapLeaves(get(layoutTree), draggedId, targetId));
      if (typeof reinitializeTerminals === 'function') {
        reinitializeTerminals();
      }
    }
  }

  function handleDragEnd(event) {
    event.target.classList.remove('dragging');
    draggedTerminalId = null;
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
