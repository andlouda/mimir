export function safelyWriteTerminal(term, data) {
  if (!term?.terminal || typeof data !== 'string' || data.length === 0) {
    return;
  }
  try {
    term.terminal.write(data);
  } catch (error) {
    console.error(`Failed to write terminal output for terminal ${term.id}:`, error);
  }
}

// PTY resizes are debounced per terminal. xterm itself is refitted at once
// (so the pane never looks stale), but the backend PTY — and with it tmux
// and the program inside — only learns the size once it has settled.
// Full-screen programs such as Claude Code redraw everything on every
// resize; feeding them each intermediate width of a divider drag or a
// quick shrink-and-grow pushes narrowly redrawn rows into the scrollback
// for good. The first size of a terminal is sent immediately.
export const RESIZE_SETTLE_MS = 300;
const pendingResizes = new Map(); // term.id → { timer, rows, cols }
const sentSizes = new Map(); // term.id → 'rows x cols'

export function safelyFitAndResizeTerminal(term, resizeTerminal) {
  if (!term || term.minimized || !term.terminal?.element) {
    return;
  }
  try {
    term.fitAddon.fit();
    const rows = Math.round(Number(term.terminal.rows));
    const cols = Math.round(Number(term.terminal.cols));
    if (Number.isInteger(rows) && rows > 0 && Number.isInteger(cols) && cols > 0) {
      scheduleResize(term.id, rows, cols, resizeTerminal);
    }
  } catch (error) {
    console.error(`Failed to fit/resize terminal ${term.id}:`, error);
  }
}

function scheduleResize(id, rows, cols, resizeTerminal) {
  const key = `${rows}x${cols}`;
  const pending = pendingResizes.get(id);
  if (pending) {
    clearTimeout(pending.timer);
    pendingResizes.delete(id);
  }
  if (!sentSizes.has(id)) {
    sentSizes.set(id, key);
    resizeTerminal(id, String(rows), String(cols));
    return;
  }
  if (sentSizes.get(id) === key) return; // back to the size the PTY already has
  const timer = setTimeout(() => {
    pendingResizes.delete(id);
    sentSizes.set(id, key);
    resizeTerminal(id, String(rows), String(cols));
  }, RESIZE_SETTLE_MS);
  pendingResizes.set(id, { timer, rows, cols });
}

/** Drops any pending resize and size memory for a terminal (on close). */
export function forgetTerminalResize(id) {
  const pending = pendingResizes.get(id);
  if (pending) clearTimeout(pending.timer);
  pendingResizes.delete(id);
  sentSizes.delete(id);
}

// observeTerminalResize refits the terminal whenever its box changes size
// (layout-tree changes, split-drag, window resize), keeping cols/rows in sync
// with the pane so content reflows instead of staying at a previous size.
//
// Two elements are observed: xterm's own root element and its current
// container (#terminal-<id>). The root alone is not enough: xterm sizes its
// root by rows/cols, so a container that only grows in height (closing a
// pane in a vertical split, dragging a horizontal divider) never changes the
// root's size and the observer would never fire — the classic "pane stays
// small" symptom. The container is re-created by Svelte on layout changes,
// so rebindTerminalResize re-observes the new one after every attach.
const resizeObservers = new Map(); // term.id → ResizeObserver

export function observeTerminalResize(term, resizeTerminal) {
  if (!term?.terminal?.element || typeof ResizeObserver === 'undefined') {
    return () => {};
  }
  let frame = 0;
  const observer = new ResizeObserver(() => {
    // Coalesce bursts (and break any fit()->resize->observe feedback) into one
    // refit per animation frame.
    if (frame) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      safelyFitAndResizeTerminal(term, resizeTerminal);
    });
  });
  resizeObservers.set(term.id, observer);
  rebindTerminalResize(term);
  return () => {
    if (frame) {
      cancelAnimationFrame(frame);
      frame = 0;
    }
    observer.disconnect();
    resizeObservers.delete(term.id);
  };
}

// rebindTerminalResize (re)targets the terminal's resize observer at xterm's
// root element and its current container. Call after every attach.
export function rebindTerminalResize(term) {
  const observer = resizeObservers.get(term?.id);
  const root = term?.terminal?.element;
  if (!observer || !root) return;
  try {
    observer.disconnect();
    observer.observe(root);
    if (root.parentElement) observer.observe(root.parentElement);
  } catch (error) {
    console.error(`Failed to observe resize for terminal ${term.id}:`, error);
  }
}

export function safelyAttachTerminal(term, element) {
  if (!term?.terminal || !element) {
    return false;
  }
  try {
    const existingTerminalElement = term.terminal.element;
    if (existingTerminalElement) {
      if (existingTerminalElement.parentElement !== element) {
        element.replaceChildren(existingTerminalElement);
      }
    } else {
      term.terminal.open(element);
    }
    return true;
  } catch (error) {
    console.error(`Failed to attach terminal ${term.id}:`, error);
    return false;
  }
}

export function safelyDisposeTerminal(term, context = 'terminal') {
  if (!term?.terminal) {
    return;
  }
  try {
    term.terminal.dispose();
  } catch (error) {
    console.error(`Failed to dispose ${context} ${term.id}:`, error);
  }
}

// Shell history hook: polyglot bash/zsh one-liner that captures each command
// via OSC 7337 escape sequence. The Go backend parses and strips these.
// Written as a semicolon-separated one-liner so it can be sent in a single
// WriteToTerminal call without multi-line issues.
export const mimirHistoryHook =
  'if [ -n "$ZSH_VERSION" ]; then ' +
    'autoload -Uz add-zsh-hook 2>/dev/null; ' +
    '__mimir_last_cmd=""; ' +
    '__mimir_precmd() { ' +
      'local exit_code=$?; ' +
      'local cmd; cmd=$(fc -ln -1 2>/dev/null); cmd=${cmd## }; ' +
      '[ -z "$cmd" ] && return; ' +
      '[ "$cmd" = "$__mimir_last_cmd" ] && return; ' +
      '__mimir_last_cmd="$cmd"; ' +
      'local b64; b64=$(printf \'%s\' "$cmd" | base64 2>/dev/null | tr -d \'\\n\'); ' +
      'printf \'\\033]7337;cmd=%s;exit=%s;cwd=%s;host=%s;user=%s;shell=zsh;ts=%s\\007\' ' +
        '"$b64" "$exit_code" "$PWD" "$(hostname -s 2>/dev/null || echo unknown)" "$(whoami)" "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"; ' +
    '}; ' +
    'add-zsh-hook precmd __mimir_precmd; ' +
  'elif [ -n "$BASH_VERSION" ]; then ' +
    '__mimir_last_cmd=""; ' +
    '__mimir_precmd() { ' +
      'local exit_code=$?; ' +
      'local cmd; cmd=$(HISTTIMEFORMAT= history 1 2>/dev/null | sed \'s/^ *[0-9]* *//\'); ' +
      '[ -z "$cmd" ] && return; ' +
      '[ "$cmd" = "$__mimir_last_cmd" ] && return; ' +
      '__mimir_last_cmd="$cmd"; ' +
      'local b64; b64=$(printf \'%s\' "$cmd" | base64 2>/dev/null | tr -d \'\\n\'); ' +
      'printf \'\\033]7337;cmd=%s;exit=%s;cwd=%s;host=%s;user=%s;shell=bash;ts=%s\\007\' ' +
        '"$b64" "$exit_code" "$PWD" "$(hostname -s 2>/dev/null || echo unknown)" "$(whoami)" "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"; ' +
    '}; ' +
    'PROMPT_COMMAND="__mimir_precmd;${PROMPT_COMMAND}"; ' +
  'fi';
