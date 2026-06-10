import { ClipboardSetText } from '../../../wailsjs/runtime';

// Write-only OSC 52 clipboard provider.
//
// tmux (set-clipboard external) reports mouse selections through OSC 52, so
// copying works even across SSH. Reading is deliberately denied: an OSC 52
// read would let any remote program exfiltrate the local clipboard.
export function createWriteOnlyClipboardProvider(setText = ClipboardSetText) {
  return {
    readText() {
      return '';
    },
    async writeText(_selection, text) {
      if (!text) return;
      await setText(text);
    },
  };
}
