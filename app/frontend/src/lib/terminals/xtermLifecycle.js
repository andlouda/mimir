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

export function safelyFitAndResizeTerminal(term, resizeTerminal) {
  if (!term || term.minimized || !term.terminal?.element) {
    return;
  }
  try {
    term.fitAddon.fit();
    const rows = Math.round(Number(term.terminal.rows));
    const cols = Math.round(Number(term.terminal.cols));
    if (Number.isInteger(rows) && rows > 0 && Number.isInteger(cols) && cols > 0) {
      resizeTerminal(term.id, String(rows), String(cols));
    }
  } catch (error) {
    console.error(`Failed to fit/resize terminal ${term.id}:`, error);
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
