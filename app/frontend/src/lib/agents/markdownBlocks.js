// Splits agent Markdown into text and fenced-code segments so the UI can
// render prose through the sanitizer and attach copy/insert actions to code.

// Any indentation: agents fence code inside nested list items.
const FENCE_OPEN = /^(\s*)(`{3,}|~{3,})\s*([^\s`]*)\s*$/;

/**
 * @param {string} markdown
 * @returns {Array<{type:'text', text:string} | {type:'code', lang:string, code:string}>}
 */
export function splitMarkdown(markdown) {
  const lines = String(markdown ?? '').split('\n');
  const segments = [];
  let text = [];
  let i = 0;

  const flushText = () => {
    const joined = text.join('\n');
    if (joined.trim()) segments.push({ type: 'text', text: joined });
    text = [];
  };

  while (i < lines.length) {
    const open = lines[i].match(FENCE_OPEN);
    if (!open) {
      text.push(lines[i]);
      i += 1;
      continue;
    }
    const fence = open[2];
    const indent = open[1].length;
    const lang = open[3] || '';
    const code = [];
    i += 1;
    let closed = false;
    while (i < lines.length) {
      const line = lines[i];
      const trimmed = line.trim();
      if (trimmed.startsWith(fence[0]) && /^[`~]+$/.test(trimmed) && trimmed[0] === fence[0] && trimmed.length >= fence.length) {
        closed = true;
        i += 1;
        break;
      }
      // Strip the fence's own indentation from the code lines.
      code.push(indent > 0 && line.startsWith(' '.repeat(indent)) ? line.slice(indent) : line);
      i += 1;
    }
    flushText();
    segments.push({ type: 'code', lang, code: code.join('\n') });
    if (!closed) break;
  }
  flushText();
  return segments;
}

/** Returns only the code segments of a Markdown string. */
export function extractCodeBlocks(markdown) {
  return splitMarkdown(markdown).filter((s) => s.type === 'code');
}

const INLINE_CODE = /`([^`\n]+)`/g;

/**
 * Extracts the copy-worthy pieces of an agent answer: fenced code blocks,
 * inline code spans that look like commands, and command lines written as
 * plain text ("$ make deploy", or an indented line under "Run:"). Pieces
 * that are already part of a block, duplicates and bare names are dropped.
 * @returns {Array<{type:'code', lang:string, code:string} | {type:'inline', code:string}>}
 */
export function extractSnippets(markdown) {
  const segments = splitMarkdown(markdown);
  const blocks = segments.filter((s) => s.type === 'code' && s.code.trim());
  const blockText = blocks.map((b) => b.code).join('\n');
  const seen = new Set();
  const inline = [];
  const add = (raw) => {
    const code = normaliseCommand(raw);
    if (code.length < 3 || seen.has(code) || blockText.includes(code)) return;
    seen.add(code);
    inline.push({ type: 'inline', code });
  };
  for (const seg of segments) {
    if (seg.type !== 'text') continue;
    for (const m of seg.text.matchAll(INLINE_CODE)) {
      if (looksLikeCommand(m[1])) add(m[1]);
    }
    for (const line of plainCommandLines(seg.text)) add(line);
  }
  return [...blocks, ...inline.slice(0, MAX_INLINE_SNIPPETS)];
}

// Prompt prefixes agents and humans put in front of a command line.
const PROMPT_PREFIX = /^\s*(?:\$|❯|PS>)\s+/;

/** Strips a shell prompt and surrounding whitespace from a command. */
export function normaliseCommand(code) {
  return String(code ?? '').replace(PROMPT_PREFIX, '').trim();
}

// Programs an agent typically tells the user to run. Plain-text lines have
// no formatting signal, so they only count when they start with one of
// these (or with a path), unlike inline code where any program word does.
const KNOWN_PROGRAMS = new Set([
  'git', 'gh', 'glab', 'npm', 'npx', 'pnpm', 'yarn', 'bun', 'deno', 'node', 'go', 'cargo', 'rustup',
  'python', 'python3', 'pip', 'pip3', 'pipx', 'poetry', 'uv', 'pytest', 'pants', 'bazel', 'make', 'cmake',
  'docker', 'podman', 'kubectl', 'helm', 'terraform', 'ansible', 'ansible-playbook', 'vagrant',
  'aws', 'gcloud', 'az', 'ssh', 'scp', 'rsync', 'curl', 'wget', 'sudo', 'cd', 'bash', 'sh', 'zsh',
  'systemctl', 'journalctl', 'tmux', 'mimir', 'claude', 'codex', 'opencode', 'mvn', 'gradle', 'dotnet',
  'java', 'ruby', 'bundle', 'rake', 'php', 'composer', 'mix', 'brew', 'apt', 'apt-get', 'dnf', 'pacman',
  'wails', 'vite', 'tsc', 'eslint', 'prettier', 'black', 'ruff', 'mypy', 'nix', 'nix-shell', 'direnv',
]);

const LABEL_LINE = /^[^\n`]{1,60}:\s*$/;

/**
 * Command lines that an agent wrote as plain text instead of Markdown code:
 * lines with a shell prompt in front, lines directly under a "Run:" style
 * label, and "Label: command" on one line. Only lines starting with a
 * well-known program or a path count, so prose is not mistaken for a command.
 */
export function plainCommandLines(text) {
  const out = [];
  const lines = String(text ?? '').split('\n');
  let underLabel = false;
  for (const raw of lines) {
    const line = raw.replace(/\s+$/, '');
    if (!line.trim()) { underLabel = false; continue; }
    if (PROMPT_PREFIX.test(line)) {
      const code = normaliseCommand(line);
      if (looksLikeCommand(code)) out.push(code);
      underLabel = false;
      continue;
    }
    if (LABEL_LINE.test(line)) { underLabel = true; continue; }
    const body = line.replace(/^\s*(?:[-*]|\d+[.)])\s+/, '').trim();
    if (underLabel || /^\s{2,}/.test(line)) {
      if (isKnownCommand(body)) out.push(body);
      // Several indented lines under one label are all commands.
      underLabel = underLabel && /^\s{2,}/.test(line);
      continue;
    }
    underLabel = false;
    const m = body.match(/^[^`:]{1,40}:\s+(\S.*)$/);
    if (m && isKnownCommand(m[1])) out.push(m[1].trim());
  }
  return out;
}

// A sentence that happens to start with a program name ("tmux ist jetzt
// die Basis, ...") reads as prose: it ends in sentence punctuation or has
// a comma or period followed by a space, which command lines rarely do.
const PROSE = /[.!?]$|[,.] /;

function isKnownCommand(code) {
  if (!looksLikeCommand(code) || PROSE.test(code)) return false;
  const first = stripEnvPrefix(normaliseCommand(code)).split(/\s+/)[0];
  return KNOWN_PROGRAMS.has(first) || /^(\.\/|\/|~\/|\.\\)/.test(first);
}

// Leading NAME=value assignments are part of a command line, not its program.
function stripEnvPrefix(code) {
  return code.replace(/^(?:[A-Za-z_][A-Za-z0-9_]*=\S*\s+)+/, '');
}

const MAX_INLINE_SNIPPETS = 8;
const MAX_LINKS = 12;

const MD_LINK = /\[([^\]\n]{1,120})\]\((https?:\/\/[^\s)]+)\)/g;
const BARE_URL = /https?:\/\/[^\s<>"'`)\]]+/g;

/**
 * Links in an agent answer, in order of appearance and without duplicates:
 * Markdown links keep their text as label, bare URLs stand alone. Trailing
 * punctuation that prose attaches to a URL is not part of it.
 * @returns {Array<{type:'link', url:string, label:string}>}
 */
export function extractLinks(markdown) {
  const text = String(markdown ?? '');
  const seen = new Set();
  const links = [];
  const add = (url, label) => {
    const clean = url.replace(/[.,;:!?]+$/, '');
    if (!clean || seen.has(clean)) return;
    seen.add(clean);
    links.push({ type: 'link', url: clean, label: label && label !== clean ? label : '' });
  };
  const found = [];
  for (const m of text.matchAll(MD_LINK)) found.push({ at: m.index, url: m[2], label: m[1].trim() });
  for (const m of text.matchAll(BARE_URL)) found.push({ at: m.index, url: m[0], label: '' });
  found.sort((a, b) => a.at - b.at);
  for (const f of found) {
    add(f.url, f.label);
    if (links.length >= MAX_LINKS) break;
  }
  return links;
}

// Inline code in prose is mostly names: files, flags, identifiers
// ("TeamSpeak.exe", "--type=utility", "network.mojom.NetworkService").
// Only something shaped like a shell command line is worth a Copy /
// Insert row: a program word followed by arguments. Besides lowercase
// Unix programs that covers env-prefixed lines ("FOO=1 make"), PowerShell
// cmdlets ("Get-ChildItem -Recurse") and scripts (".\\build.ps1 -Clean").
// "Label: value" pairs ("deploy stack: ai-agents") are prose, not commands.
export function looksLikeCommand(code) {
  const cmd = stripEnvPrefix(normaliseCommand(code));
  if (!/\s/.test(cmd)) return false;
  if (/[\n\r]/.test(cmd)) return false;
  const words = cmd.split(/\s+/);
  const first = words[0];
  if (words.slice(0, 2).some((w) => /^[^:]+:$/.test(w))) return false;
  const unix = /^(\.\/|\/|~\/)?[a-z][a-z0-9_.+\/-]*$/.test(first);
  const powershell = /^([A-Z][a-z]+-[A-Z][A-Za-z]+|\.\\[\w.\\-]+|&)$/.test(first);
  if (!unix && !powershell) return false;
  if (/^[a-z]+\.(exe|dll|js|ts|go|py|json|md)$/i.test(first)) return false;
  return true;
}

/**
 * First lines of prose of an answer, with code blocks removed and the length
 * capped, for a "what did it just say" summary.
 */
export function firstProse(markdown, maxChars = 320) {
  const prose = splitMarkdown(markdown)
    .filter((s) => s.type === 'text')
    .map((s) => s.text)
    .join('\n')
    .replace(/^#+\s*/gm, '')
    .replace(/\*\*/g, '')
    .replace(/\n{2,}/g, '\n')
    .trim();
  if (prose.length <= maxChars) return prose;
  const cut = prose.slice(0, maxChars);
  const lastSpace = cut.lastIndexOf(' ');
  return (lastSpace > maxChars * 0.6 ? cut.slice(0, lastSpace) : cut) + '…';
}

/**
 * Groups a transcript into turns: one user prompt followed by every
 * assistant message until the next prompt. Agents split a single reply
 * across several records (text, tool call, text, ...), so "the last answer"
 * must mean the whole last turn, not the last record.
 * @param {Array<{role:string, text:string, timestamp?:string}>} messages
 * @returns {Array<{prompt: object|null, answers: object[]}>}
 */
export function groupTurns(messages) {
  const turns = [];
  let current = null;
  for (const message of messages || []) {
    if (message.role === 'user') {
      current = { prompt: message, answers: [] };
      turns.push(current);
      continue;
    }
    if (!current) {
      current = { prompt: null, answers: [] };
      turns.push(current);
    }
    current.answers.push(message);
  }
  return turns.filter((t) => t.answers.length > 0);
}
