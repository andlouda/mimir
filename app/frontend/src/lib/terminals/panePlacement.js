// Where a new pane goes. Side by side ("row") narrows every pane in the
// row; programs that wrap their own output (Claude Code and other TUIs)
// keep those narrow lines forever, because they write hard line breaks.
// Stacking ("column") costs rows instead of columns, which such output
// does not mind. "auto" stacks only while an agent is visible.

import { get } from 'svelte/store';
import { terminals } from '../stores/terminalStore.js';
import { agentStates } from '../stores/agentStore.js';
import { panePlacement } from '../stores/uiStore.js';

export const PANE_PLACEMENTS = ['row', 'column', 'auto'];

/** Split direction for a new pane per the placement setting and the open panes. */
export function newPaneDirection() {
  return placementDirection(get(panePlacement), { visibleAgent: hasVisibleAgent(get(terminals), get(agentStates)) });
}

/**
 * @param {string} placement 'auto' | 'row' | 'column'
 * @param {{ visibleAgent: boolean }} ctx whether a visible pane runs an agent
 * @returns {'horizontal' | 'vertical'} split direction for appendLeaf
 */
export function placementDirection(placement, { visibleAgent = false } = {}) {
  if (placement === 'column') return 'vertical';
  if (placement === 'auto') return visibleAgent ? 'vertical' : 'horizontal';
  return 'horizontal';
}

/** True when any non-minimized terminal has a detected agent. */
export function hasVisibleAgent(terminals, agentStates) {
  return (terminals || []).some((t) => !t.minimized && agentStates && agentStates[t.id]);
}
