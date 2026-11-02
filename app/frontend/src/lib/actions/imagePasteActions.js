import { get } from 'svelte/store';
import { SavePastedImage } from '../backend.js';
import { t } from '../i18n.js';
import { errorMessage } from '../stores/uiStore.js';
import { blobToBase64 } from '../terminals/clipboardImage.js';
import { quotePathFor } from '../terminals/droppedPaths.js';

// Uploads a pasted image to where the pane's program runs and pastes the
// path, quoted for that shell, without a newline: the user still decides
// what to do with it. The backend stores it in the host's cache directory.
export async function pasteImageAsFile(term, file, { save = SavePastedImage } = {}) {
  try {
    const data = await blobToBase64(file);
    const path = await save(term.id, file.type, data);
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
