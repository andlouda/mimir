import { optional } from '../backend.js';
import { isWindowsPlatform } from './terminalTypes.js';

// xterm's ConPTY mode for local panes on Windows.
//
// ConPTY renders the screen itself. When a pane grows by N rows, ConPTY pads
// the bottom with empty rows and keeps the cursor where it was, while xterm
// would pull N scrollback lines back into the viewport. Both at once leave
// every later absolute cursor move N rows off: a TUI such as Claude Code
// draws its input box into rows that are not there and the bottom of the
// pane stays dark. With `windowsPty` set, xterm follows ConPTY. The build
// number lets xterm keep line reflow on builds whose ConPTY marks wrapped
// lines (21376 and later). SSH panes talk to a remote PTY and stay as they
// are.

let buildNumberPromise = null;

// Resolves to the Windows build number, or undefined when the backend does
// not report one. Asked once per page load.
export function windowsBuildNumber() {
  if (!buildNumberPromise) {
    const fn = optional('GetWindowsBuildNumber');
    buildNumberPromise = fn
      ? Promise.resolve()
          .then(() => fn())
          .then((n) => (Number.isInteger(n) && n > 0 ? n : undefined))
          .catch(() => undefined)
      : Promise.resolve(undefined);
  }
  return buildNumberPromise;
}

export function resetWindowsBuildNumber() {
  buildNumberPromise = null;
}

// The `windowsPty` option for a terminal of the given type, or undefined when
// the pane is not backed by ConPTY.
export async function windowsPtyOptions(type, { windows = isWindowsPlatform } = {}) {
  if (!windows || type === 'ssh') return undefined;
  const buildNumber = await windowsBuildNumber();
  return buildNumber ? { backend: 'conpty', buildNumber } : { backend: 'conpty' };
}
