// Generic, state-free helpers extracted from App.svelte.

/** Single-quotes a path for safe interpolation into a POSIX shell command. */
export function shellQuotePath(path) {
  return `'${String(path).replace(/'/g, `'\\''`)}'`;
}

/**
 * Sanitizes HTML generated from local or imported Markdown before rendering it
 * with Svelte's {@html}. This is a security boundary: Wails exposes backend
 * methods to the frontend, so imported Markdown must not execute script.
 */
export function sanitizeHtml(html) {
  if (!html) return '';
  if (typeof DOMParser === 'undefined') return '';

  const allowedTags = new Set([
    'a', 'abbr', 'blockquote', 'br', 'code', 'del', 'details', 'div', 'em',
    'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'hr', 'img', 'kbd', 'li', 'ol', 'p',
    'pre', 's', 'span', 'strong', 'sub', 'summary', 'sup', 'table', 'tbody',
    'td', 'th', 'thead', 'tr', 'ul'
  ]);
  const globalAttrs = new Set(['class', 'title']);
  const tagAttrs = {
    a: new Set(['href', 'rel', 'target']),
    img: new Set(['src', 'alt', 'width', 'height'])
  };
  const uriAttrs = new Set(['href', 'src']);
  const safeSchemes = new Set(['http:', 'https:', 'mailto:']);

  const parser = new DOMParser();
  const doc = parser.parseFromString(`<body>${html}</body>`, 'text/html');

  function isSafeUrl(value) {
    const trimmed = String(value || '').trim();
    if (!trimmed) return false;
    if (trimmed.startsWith('#') || trimmed.startsWith('/')) return true;
    try {
      return safeSchemes.has(new URL(trimmed, window.location.href).protocol);
    } catch {
      return false;
    }
  }

  function cleanNode(node) {
    if (node.nodeType === Node.COMMENT_NODE) {
      node.remove();
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;

    const tag = node.tagName.toLowerCase();
    if (!allowedTags.has(tag)) {
      node.replaceWith(...Array.from(node.childNodes));
      return;
    }

    for (const attr of Array.from(node.attributes)) {
      const name = attr.name.toLowerCase();
      const allowed = globalAttrs.has(name) || tagAttrs[tag]?.has(name);
      if (!allowed || name.startsWith('on')) {
        node.removeAttribute(attr.name);
        continue;
      }
      if (uriAttrs.has(name) && !isSafeUrl(attr.value)) {
        node.removeAttribute(attr.name);
      }
    }

    if (tag === 'a') {
      node.setAttribute('rel', 'noreferrer noopener');
      if (node.getAttribute('target') === '_blank') {
        node.setAttribute('target', '_blank');
      }
    }
  }

  let current;
  const walker = doc.createTreeWalker(doc.body, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_COMMENT);
  const nodes = [];
  while ((current = walker.nextNode())) nodes.push(current);
  for (const node of nodes) cleanNode(node);
  return doc.body.innerHTML;
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
