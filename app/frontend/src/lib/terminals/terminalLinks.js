// URL handling inside terminals: xterm's web-links addon underlines URLs on
// hover; Ctrl/Cmd+click opens them in the system browser, the context menu
// offers open/copy for the URL under the pointer. Only http(s) is ever
// opened: terminal output is untrusted and other schemes could reach local
// handlers.
import { WebLinksAddon } from '@xterm/addon-web-links';
import { BrowserOpenURL } from '../../../wailsjs/runtime';

const hovered = new Map(); // terminal id → url under the pointer

export function isOpenableUrl(url) {
  try {
    const parsed = new URL(String(url));
    return parsed.protocol === 'http:' || parsed.protocol === 'https:';
  } catch {
    return false;
  }
}

export function openUrl(url) {
  if (!isOpenableUrl(url)) return false;
  try {
    BrowserOpenURL(url);
    return true;
  } catch (error) {
    console.error('Failed to open URL:', error);
    return false;
  }
}

/** URL currently under the pointer in a terminal, or ''. */
export function hoveredLink(terminalId) {
  return hovered.get(terminalId) || '';
}

export function clearHoveredLink(terminalId) {
  hovered.delete(terminalId);
}

/** Creates the addon for one terminal; load it with terminal.loadAddon(). */
export function createWebLinksAddon(terminalId, { open = openUrl } = {}) {
  return new WebLinksAddon(
    (event, uri) => {
      // Plain clicks go to the application (or place the cursor); a modifier
      // makes the intent explicit, like in GNOME Terminal / Windows Terminal.
      if (event?.ctrlKey || event?.metaKey) open(uri);
    },
    {
      hover: (_event, text) => hovered.set(terminalId, text),
      leave: () => hovered.delete(terminalId),
    },
  );
}
