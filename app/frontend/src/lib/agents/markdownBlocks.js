// Splits agent Markdown into text and fenced-code segments so the UI can
// render prose through the sanitizer and attach copy/insert actions to code.

const FENCE_OPEN = /^(\s{0,3})(`{3,}|~{3,})\s*([^\s`]*)\s*$/;

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
 * Extracts the copy-worthy pieces of an agent answer: fenced code blocks and
 * inline code spans (commands, paths, flags). Inline spans that are already
 * part of a block, are duplicates, or are too short to be useful are dropped.
 * @returns {Array<{type:'code', lang:string, code:string} | {type:'inline', code:string}>}
 */
export function extractSnippets(markdown) {
  const segments = splitMarkdown(markdown);
  const blocks = segments.filter((s) => s.type === 'code' && s.code.trim());
  const blockText = blocks.map((b) => b.code).join('\n');
  const seen = new Set();
  const inline = [];
  for (const seg of segments) {
    if (seg.type !== 'text') continue;
    for (const m of seg.text.matchAll(INLINE_CODE)) {
      const code = m[1].trim();
      if (code.length < 3 || seen.has(code) || blockText.includes(code)) continue;
      seen.add(code);
      inline.push({ type: 'inline', code });
    }
  }
  return [...blocks, ...inline];
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
