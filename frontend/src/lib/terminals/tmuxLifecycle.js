export function generateTmuxSessionName(prefix = 'mimir') {
  const bytes = new Uint8Array(4);
  crypto.getRandomValues(bytes);
  const normalizedPrefix = String(prefix || 'mimir')
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '') || 'mimir';
  return `${normalizedPrefix}-${Array.from(bytes).map((b) => b.toString(16).padStart(2, '0')).join('')}`;
}

export const histPrimer = ' HISTCONTROL=ignoreboth; setopt HIST_IGNORE_SPACE 2>/dev/null; set +o history 2>/dev/null; builtin history -d -1 2>/dev/null || builtin history -d $HISTCMD 2>/dev/null; true\r';
export const histOff = ' set +o history 2>/dev/null; HISTCONTROL=ignoreboth; setopt HIST_IGNORE_SPACE 2>/dev/null; fc -p /dev/null 2>/dev/null;';
export const histOn = '; fc -P 2>/dev/null; set -o history 2>/dev/null';

export function buildSSHTmuxCommand(sessionName, reattaching) {
  if (reattaching) {
    return `tmux new-session -A -s ${sessionName} \\; set status off \\; set prefix None \\; set prefix2 None`;
  }
  return `tmux new-session -s ${sessionName} \\; set status off \\; set prefix None \\; set prefix2 None`;
}

export function buildLocalTmuxCommand(sessionName, reattaching) {
  if (reattaching) {
    return `tmux -L mimir new-session -A -s ${sessionName} \\; set status off \\; set prefix None \\; set prefix2 None`;
  }
  return `tmux -L mimir new-session -s ${sessionName} \\; set status off \\; set prefix None \\; set prefix2 None`;
}
