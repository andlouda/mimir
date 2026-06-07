// Pure cleaning pipeline for terminal transcripts. Lives outside any Svelte
// component so it can be unit-tested in isolation and reused by other surfaces
// (e.g. recording exports, AI context windows) without dragging the modal in.
//
// Pipeline order matters:
//   1. stripAnsi       — remove control sequences first so the next passes work
//                        on plain bytes.
//   2. applyCarriageReturns — collapse cursor rewinds within a line.
//   3. collapseRepeats — fold blank-line runs and long identical runs.
//
// Each step is exported individually for targeted testing; cleanTranscript
// composes them in the right order.

const CSI_RE = /\x1B\[[\d;?<>]*[a-zA-Z]/g;
const OSC_RE = /\x1B\][^\x07\x1B]*(?:\x07|\x1B\\)/g;
const ESC_RE = /\x1B[=>()][AB012]?/g;
const CTRL_RE = /[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]/g;

const BLANK_LINE_RE = /^\s*$/;

// Identical non-blank lines only collapse to a marker when there are at least
// this many in a row. Below the threshold, all repeats are emitted verbatim —
// "1× repeated" is louder than the line itself.
export const REPEAT_MARKER_MIN = 4;

/**
 * Strip ANSI escape sequences and unsafe control bytes. Preserves TAB (0x09),
 * LF (0x0A), and CR (0x0D) — the latter is consumed by applyCarriageReturns.
 */
export function stripAnsi(text) {
  if (!text) return '';
  return text
    .replace(CSI_RE, '')
    .replace(OSC_RE, '')
    .replace(ESC_RE, '')
    .replace(CTRL_RE, '');
}

/**
 * Resolve carriage returns the way a real terminal would: within a single
 * line, content before the last \r is overwritten by what comes after. Lines
 * without \r are untouched. Without this, shells that redraw the prompt on
 * every keystroke (PSReadLine, fancy zsh themes) produce glued-together
 * fragments like "ppwpwd" from "p\rpw\rpwd".
 */
export function applyCarriageReturns(text) {
  if (!text || !text.includes('\r')) return text;
  return text
    .split('\n')
    .map((line) => {
      if (!line.includes('\r')) return line;
      const segments = line.split('\r');
      return segments[segments.length - 1];
    })
    .join('\n');
}

function isBlankLine(s) {
  return s.length === 0 || BLANK_LINE_RE.test(s);
}

/**
 * Squash noisy repetition without obscuring real content:
 *   - runs of blank lines collapse to a single blank (no marker)
 *   - runs of >= REPEAT_MARKER_MIN identical non-blank lines collapse to one
 *     copy of the line plus a marker; shorter runs are kept verbatim
 */
export function collapseRepeats(text) {
  const lines = text.split('\n');
  const out = [];
  let prev = null;
  let runCount = 0;

  const flush = () => {
    if (prev === null) return;
    if (isBlankLine(prev)) {
      out.push(prev);
      return;
    }
    if (runCount >= REPEAT_MARKER_MIN) {
      out.push(prev);
      out.push(`  ⟨${runCount - 1}× more identical⟩`);
      return;
    }
    for (let i = 0; i < runCount; i++) out.push(prev);
  };

  for (const line of lines) {
    if (line === prev) {
      runCount += 1;
      continue;
    }
    flush();
    prev = line;
    runCount = 1;
  }
  flush();
  return out.join('\n');
}

/**
 * Apply the full clean-view pipeline. Equivalent to:
 *   collapseRepeats(applyCarriageReturns(stripAnsi(text)))
 */
export function cleanTranscript(text) {
  return collapseRepeats(applyCarriageReturns(stripAnsi(text)));
}
