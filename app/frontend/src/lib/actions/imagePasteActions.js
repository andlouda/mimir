import { get } from 'svelte/store';
import { SavePastedImage } from '../backend.js';
import { t } from '../i18n.js';
import { errorMessage } from '../stores/uiStore.js';
import { blobToBase64 } from '../terminals/clipboardImage.js';
import { quotePathFor } from '../terminals/droppedPaths.js';

// The image behind a paste event, read through the async clipboard API
// when the event carried none. WebView2 announces an image on the Windows
// clipboard in the event's types but does not always hand over a File;
// navigator.clipboard.read() returns the PNG the browser renders from the
// bitmap. null when the API is missing, denied or holds no image.
export async function readClipboardImage({ clipboard = globalThis.navigator?.clipboard } = {}) {
  if (!clipboard || typeof clipboard.read !== 'function') return null;
  try {
    for (const item of await clipboard.read()) {
      const type = (item.types || []).find((tp) => typeof tp === 'string' && tp.startsWith('image/'));
      if (type) return await item.getType(type);
    }
  } catch (error) {
    console.warn('clipboard.read() failed:', error?.message || error);
  }
  return null;
}

// Uploads a pasted image to where the pane's program runs and pastes the
// path, quoted for that shell, without a newline: the user still decides
// what to do with it. The backend stores it in the host's cache directory.
// Never falls back to a keystroke: a Ctrl+V on an SSH host only makes the
// program look for a clipboard it cannot see, so a failure is reported.
export async function pasteImageAsFile(term, file, { save = SavePastedImage, types = [], readClipboard = readClipboardImage } = {}) {
  try {
    const blob = file || await readClipboard();
    if (!blob) {
      errorMessage.set(get(t)('appMessages.imagePasteNoFile', { types: types.join(', ') || '-' }));
      return '';
    }
    const data = await blobToBase64(blob);
    const path = await save(term.id, blob.type, data);
    if (!path) throw new Error('no path returned');
    // An SSH host runs a POSIX shell; local panes keep their own quoting.
    term.terminal.paste(quotePathFor(path, term.type === 'ssh' ? '' : term.type));
    return path;
  } catch (error) {
    console.error(`Image paste failed for terminal ${term?.id}:`, error);
    errorMessage.set(get(t)('appMessages.imagePasteFailed', { error: error?.message || String(error) }));
    return '';
  }
}
