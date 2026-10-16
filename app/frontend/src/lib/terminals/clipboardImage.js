// A paste whose clipboard holds no text but an image. The programs that
// take images (Claude Code, OpenCode) read them from the system clipboard
// themselves when they see Ctrl+V, so the pane only needs the keystroke.
export const CTRL_V = '\x16';

export function clipboardHasImage(data) {
  if (!data) return false;
  const isImage = (type) => typeof type === 'string' && type.startsWith('image/');
  if (Array.from(data.types || []).some(isImage)) return true;
  if (Array.from(data.items || []).some((item) => item?.kind === 'file' && isImage(item.type))) return true;
  return Array.from(data.files || []).some((file) => isImage(file?.type));
}

// Decides what a paste event means for the pane: text to paste, an image
// to hand to the program as Ctrl+V, or nothing.
export function pasteAction(data) {
  const text = data?.getData?.('text') || '';
  if (text) return { kind: 'text', text };
  if (clipboardHasImage(data)) return { kind: 'image' };
  return { kind: 'none' };
}
