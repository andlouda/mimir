import { get } from 'svelte/store';
import { SetHistoryTracking } from '../backend.js';
import { historyTrackingEnabled } from '../stores/uiStore.js';

// Flips the local command-history consent; the store follows the backend.
export async function toggleHistoryTracking() {
  const next = !get(historyTrackingEnabled);
  await SetHistoryTracking(next);
  historyTrackingEnabled.set(next);
}
