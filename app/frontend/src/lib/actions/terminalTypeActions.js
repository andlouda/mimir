import { get } from 'svelte/store';
import { GetAvailableTerminalTypes } from '../backend.js';
import { availableTerminalTypes, selectedTerminalType } from '../stores/terminalStore.js';
import { DEFAULT_TERMINAL_TYPE_KEY, preferredTerminalType } from '../terminals/terminalTypes.js';

export function persistDefaultTerminalType() {
  try {
    localStorage.setItem(DEFAULT_TERMINAL_TYPE_KEY, get(selectedTerminalType));
  } catch (error) {
    console.error('Failed to save default terminal type:', error);
  }
}

// Replaces the static list with what the backend found installed; a
// remembered default that is no longer available falls back to the
// platform's preferred shell.
export async function loadAvailableTerminalTypes() {
  try {
    const detected = await GetAvailableTerminalTypes();
    if (Array.isArray(detected) && detected.length > 0) {
      availableTerminalTypes.set(detected);
      if (!detected.some((option) => option.value === get(selectedTerminalType))) {
        selectedTerminalType.set(preferredTerminalType(detected).value);
        persistDefaultTerminalType();
      }
    }
  } catch (error) {
    console.error('Failed to load terminal types:', error);
  }
}
