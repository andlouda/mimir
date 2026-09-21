// URL handling inside terminals: URLs are underlined on hover, Ctrl/Cmd+click
// opens them in the system browser, the context menu offers open/copy for the
// URL under the pointer. Detection joins URLs that a program wrapped across
// rows (see multiRowLinks.js). Only http(s) is ever opened: terminal output
// is untrusted and other schemes could reach local handlers.
import { BrowserOpenURL } from '../../../wailsjs/runtime';
import { findLinksAt } from './multiRowLinks.js';

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

/**
 * Creates an xterm link provider for one terminal; register it with
 * terminal.registerLinkProvider() and dispose the result on cleanup.
 */
export function createLinkProvider(terminalId, terminal, { open = openUrl } = {}) {
  const rowText = (y) => {
    const line = terminal.buffer?.active?.getLine?.(y - 1);
    return line ? line.translateToString(true) : null;
  };
  return {
    provideLinks(y, callback) {
      let found = [];
      try {
        found = findLinksAt(rowText, y, terminal.cols || 0);
      } catch (error) {
        console.error('Link detection failed:', error);
      }
      if (!found.length) {
        callback(undefined);
        return;
      }
      callback(found.map((link) => ({
        range: { start: link.start, end: link.end },
        text: link.text,
        decorations: { underline: true, pointerCursor: true },
        activate: (event, text) => {
          // Plain clicks go to the application (or place the cursor); a
          // modifier makes the intent explicit, like in GNOME Terminal.
          if (event?.ctrlKey || event?.metaKey) open(text);
        },
        hover: (_event, text) => hovered.set(terminalId, text),
        leave: () => hovered.delete(terminalId),
      })));
    },
  };
}
