// A paste whose clipboard holds no text but an image. The programs that
// take images (Claude Code, OpenCode) read them from the system clipboard
// themselves when they see Ctrl+V, so a local pane only needs the
// keystroke. A program on an SSH host cannot see this clipboard: there the
// image goes up as a file and its path is pasted (actions/imagePasteActions.js).
export const CTRL_V = '\x16';

const isImage = (type) => typeof type === 'string' && type.startsWith('image/');

export function clipboardHasImage(data) {
  if (!data) return false;
  if (Array.from(data.types || []).some(isImage)) return true;
  if (Array.from(data.items || []).some((item) => item?.kind === 'file' && isImage(item.type))) return true;
  return Array.from(data.files || []).some((file) => isImage(file?.type));
}

// The image on the clipboard as a File, or null when the browser only
// announced the type without handing over the bytes.
export function clipboardImageFile(data) {
  for (const item of Array.from(data?.items || [])) {
    if (item?.kind === 'file' && isImage(item.type)) {
      const file = item.getAsFile?.();
      if (file) return file;
    }
  }
  return Array.from(data?.files || []).find((file) => isImage(file?.type)) || null;
}

// Decides what a paste event means for the pane: text to paste, an image
// (with its file when available), or nothing.
export function pasteAction(data) {
  const text = data?.getData?.('text') || '';
  if (text) return { kind: 'text', text };
  if (clipboardHasImage(data)) return { kind: 'image', file: clipboardImageFile(data) };
  return { kind: 'none' };
}

// How a pane gets an image: 'keystroke' hands the program Ctrl+V so it
// reads the clipboard itself; 'file' stores the image and pastes its path.
// SSH panes always take the file; local panes follow the setting.
export function imagePasteMode(term, localSetting = 'clipboard') {
  if (term?.type === 'ssh') return 'file';
  return localSetting === 'file' ? 'file' : 'keystroke';
}

// Base64 of a Blob, chunked so a multi-megabyte screenshot does not blow
// the argument limit of String.fromCharCode.
export async function blobToBase64(blob) {
  const bytes = new Uint8Array(await blob.arrayBuffer());
  let binary = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}
