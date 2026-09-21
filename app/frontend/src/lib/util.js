// Generic, state-free helpers extracted from App.svelte.
import DOMPurify from 'dompurify';

/** Single-quotes a path for safe interpolation into a POSIX shell command. */
export function shellQuotePath(path) {
  return `'${String(path).replace(/'/g, `'\\''`)}'`;
}

/**
 * True when the string contains a C0 control character or DEL. Paths and
 * names that end up as keystrokes in a terminal must never carry these: a CR
 * submits a command early and ESC sequences can trigger readline bindings.
 */
export function containsControlChars(value) {
  // eslint-disable-next-line no-control-regex
  return /[\x00-\x1f\x7f]/.test(String(value ?? ''));
}

const SANITIZE_ALLOWED_TAGS = [
  'a', 'abbr', 'blockquote', 'br', 'code', 'del', 'details', 'div', 'em',
  'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'hr', 'img', 'kbd', 'li', 'ol', 'p',
  'pre', 's', 'span', 'strong', 'sub', 'summary', 'sup', 'table', 'tbody',
  'td', 'th', 'thead', 'tr', 'ul'
];
const SANITIZE_ALLOWED_ATTRS = ['class', 'title', 'href', 'rel', 'target', 'src', 'alt', 'width', 'height'];
// Remote images are blocked on purpose: an <img src="https://..."> in an
// imported note is a tracking beacon and extra WebKit attack surface. Only
// inline raster images survive.
const SANITIZE_IMG_SRC = /^data:image\/(?:png|jpe?g|gif|webp|bmp);base64,[a-z0-9+/=\s]+$/i;
const SANITIZE_LINK_SCHEMES = new Set(['http:', 'https:', 'mailto:']);

let purifier = null;

function getPurifier() {
  if (purifier) return purifier;
  if (typeof window === 'undefined' || typeof DOMParser === 'undefined') return null;
  const instance = DOMPurify(window);
  if (!instance.isSupported) return null;
  instance.setConfig({
    ALLOWED_TAGS: SANITIZE_ALLOWED_TAGS,
    ALLOWED_ATTR: SANITIZE_ALLOWED_ATTRS,
    ALLOW_DATA_ATTR: false,
    ALLOW_ARIA_ATTR: false,
    ALLOW_UNKNOWN_PROTOCOLS: false,
    ALLOWED_URI_REGEXP: /^(?:https?:|mailto:|data:image\/|#|\/)/i,
    // Non-URI attributes DOMPurify would otherwise validate against the URI
    // regexp above. href/src are still checked by the hook below.
    ADD_URI_SAFE_ATTR: ['target', 'rel', 'width', 'height'],
    SAFE_FOR_TEMPLATES: false,
    RETURN_DOM: false,
    RETURN_DOM_FRAGMENT: false,
    WHOLE_DOCUMENT: false,
  });
  instance.addHook('uponSanitizeAttribute', (node, data) => {
    const tag = node.nodeName.toLowerCase();
    const name = data.attrName;
    if (name === 'href' || name === 'src' || name === 'target' || name === 'rel') {
      if (tag !== 'a' && tag !== 'img') {
        data.keepAttr = false;
        return;
      }
    }
    if ((name === 'alt' || name === 'width' || name === 'height') && tag !== 'img') {
      data.keepAttr = false;
      return;
    }
    if (tag === 'img' && name === 'src') {
      data.keepAttr = SANITIZE_IMG_SRC.test(String(data.attrValue || '').trim());
      return;
    }
    if (tag === 'a' && name === 'href') {
      const value = String(data.attrValue || '').trim();
      if (value.startsWith('#') || value.startsWith('/')) return;
      try {
        data.keepAttr = SANITIZE_LINK_SCHEMES.has(new URL(value, window.location.href).protocol);
      } catch {
        data.keepAttr = false;
      }
    }
  });
  instance.addHook('afterSanitizeAttributes', (node) => {
    if (node.nodeName.toLowerCase() !== 'a') return;
    node.setAttribute('rel', 'noreferrer noopener');
    if (node.getAttribute('target') !== '_blank') {
      node.removeAttribute('target');
    }
  });
  purifier = instance;
  return purifier;
}

/**
 * Sanitizes HTML generated from local or imported Markdown before rendering it
 * with Svelte's {@html}. This is a security boundary: Wails exposes backend
 * methods (WriteToTerminal, SFTP, ...) to the frontend, so imported Markdown
 * must not execute script. DOMPurify does the parsing/mutation-safe work; the
 * config above restricts it to the Markdown subset the notes need.
 *
 * Fails closed: without a usable DOM (SSR, tests without jsdom) it returns an
 * empty string rather than unsanitized markup.
 */
export function sanitizeHtml(html) {
  if (!html) return '';
  const instance = getPurifier();
  if (!instance) return '';
  return instance.sanitize(String(html));
}

/**
 * Joins a multi-line terminal selection into one line, undoing wraps that a
 * program drew itself (Claude Code, Codex): a row that fills the terminal
 * width was cut mid-token and is glued to the next row without a space
 * (URLs, paths, tokens); a shorter row was wrapped at a word boundary and
 * gets a single space. Continuation indentation is dropped.
 */
export function joinSelectionLines(text, cols) {
  const lines = String(text ?? '').replace(/\r/g, '').split('\n');
  if (lines.length <= 1) return lines[0] || '';
  let out = lines[0].trimEnd();
  for (let i = 1; i < lines.length; i++) {
    const next = lines[i].trim();
    if (!next) continue;
    const prevFull = cols > 0 && lines[i - 1].length >= cols;
    out += prevFull ? next : ' ' + next;
  }
  return out;
}

/** Generates a unique resume id for a terminal session. */
export function generateResumeId() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `resume-${Date.now()}-${Math.random().toString(16).slice(2, 10)}`;
}

/** Removes duplicate saved-session terminals (by resumeId, else by a composite key). */
export function dedupeSavedSessionTerminals(savedTerminals) {
  const seen = new Set();
  const result = [];

  for (const saved of savedTerminals || []) {
    if (!saved || !saved.type) continue;
    const key = saved.resumeId
      ? `resume:${saved.resumeId}`
      : [
          saved.type,
          saved.name || '',
          saved.sshProfileId || '',
          saved.tmuxSessionName || '',
          saved.minimized ? 'min' : 'vis'
        ].join('|');
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(saved);
  }

  return result;
}

/**
 * Cleans a raw terminal transcript for preview: strips ANSI/control sequences,
 * normalizes whitespace, drops duplicate lines/blocks, and returns the last few
 * blocks. State-free.
 */
export function sanitizeTranscriptPreview(raw) {
  if (!raw) return '';

  let text = String(raw);

  // Strip ANSI CSI sequences.
  text = text.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, '');
  // Strip ANSI OSC sequences.
  text = text.replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, '');
  // Strip stray title-like fragments that may survive chunk boundaries.
  text = text.replace(/(?:^|\n)\]0;[^\n]*/g, '\n');
  // Strip remaining non-printing control chars except newlines and tabs.
  text = text.replace(/[\x00-\x08\x0b-\x1f\x7f]/g, '');
  // Normalize blank lines.
  text = text.replace(/\r\n/g, '\n');
  text = text.replace(/\r/g, '\n');
  text = text.replace(/\t/g, '  ');
  text = text.replace(/^\[K$/gm, '');
  text = text.replace(/\n{3,}/g, '\n\n');
  text = text.trim();

  if (!text) return '';
  const normalizeLine = (line) => line.replace(/[  ]+/g, ' ').trim();
  const rawLines = text.split('\n').map(normalizeLine);

  const dedupedLines = [];
  for (const line of rawLines) {
    if (!line) {
      const previous = dedupedLines[dedupedLines.length - 1];
      if (previous === '') {
        continue;
      }
      dedupedLines.push(line);
      continue;
    }
    if (/^Microsoft Windows \[Version .+\]$/i.test(line)) {
      continue;
    }
    if (/^\[[A-Z]\]$/i.test(line)) {
      continue;
    }
    const previous = dedupedLines[dedupedLines.length - 1];
    if (line !== '' && line === previous) {
      continue;
    }
    dedupedLines.push(line);
  }

  const blocks = [];
  let currentBlock = [];
  for (const line of dedupedLines) {
    if (line === '') {
      if (currentBlock.length > 0) {
        blocks.push(currentBlock.join('\n'));
        currentBlock = [];
      }
      continue;
    }
    currentBlock.push(line);
  }
  if (currentBlock.length > 0) {
    blocks.push(currentBlock.join('\n'));
  }

  const dedupedBlocks = [];
  for (const block of blocks) {
    const previous = dedupedBlocks[dedupedBlocks.length - 1];
    if (block && block !== previous) {
      dedupedBlocks.push(block);
    }
  }

  const tailBlocks = dedupedBlocks.slice(-3);
  const tail = tailBlocks.join('\n\n').trim();
  return tail;
}
