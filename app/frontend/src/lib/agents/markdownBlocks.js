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
