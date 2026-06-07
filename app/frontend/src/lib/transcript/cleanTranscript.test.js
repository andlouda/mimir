import { describe, expect, it } from 'vitest';
import {
  REPEAT_MARKER_MIN,
  applyCarriageReturns,
  cleanTranscript,
  collapseRepeats,
  stripAnsi,
} from './cleanTranscript.js';

describe('stripAnsi', () => {
  it('removes CSI color sequences', () => {
    const input = '\x1B[31mred\x1B[0m\x1B[1;32mgreen\x1B[0m';
    expect(stripAnsi(input)).toBe('redgreen');
  });

  it('removes DEC private-mode toggles like \\x1B[?25l', () => {
    const input = '\x1B[?25lhello\x1B[?25h';
    expect(stripAnsi(input)).toBe('hello');
  });

  it('removes OSC title pushes terminated by BEL', () => {
    const input = '\x1B]0;user@host: ~\x07prompt$ ';
    expect(stripAnsi(input)).toBe('prompt$ ');
  });

  it('removes OSC sequences terminated by ESC\\', () => {
    const input = '\x1B]133;A\x1B\\command';
    expect(stripAnsi(input)).toBe('command');
  });

  it('removes ESC charset-selection like \\x1B(B', () => {
    expect(stripAnsi('\x1B(Bplain')).toBe('plain');
  });

  it('removes C0 control bytes except TAB/LF/CR', () => {
    const input = 'a\x00b\x07c\tcol\nrow\rretkeep';
    expect(stripAnsi(input)).toBe('abc\tcol\nrow\rretkeep');
  });

  it('returns empty string for null/undefined/empty input', () => {
    expect(stripAnsi('')).toBe('');
    expect(stripAnsi(undefined)).toBe('');
    expect(stripAnsi(null)).toBe('');
  });

  it('leaves text without any escapes untouched', () => {
    const input = 'no escapes here\nplain text\n';
    expect(stripAnsi(input)).toBe(input);
  });
});

describe('applyCarriageReturns', () => {
  it('keeps only the segment after the last \\r within a line', () => {
    // Simulates PSReadLine redrawing the prompt as the user types "pwd".
    expect(applyCarriageReturns('t3 ❯ p\rt3 ❯ pw\rt3 ❯ pwd')).toBe('t3 ❯ pwd');
  });

  it('processes each line independently', () => {
    const input = 'a\rab\rabc\nb\rbb\rbbb';
    expect(applyCarriageReturns(input)).toBe('abc\nbbb');
  });

  it('returns the input unchanged when there is no \\r', () => {
    const input = 'line one\nline two\n';
    expect(applyCarriageReturns(input)).toBe(input);
  });

  it('handles trailing \\r before \\n by keeping the segment after it', () => {
    // A common pattern: shells emit "text\r\n" rather than just "\n".
    expect(applyCarriageReturns('text\r\nnext')).toBe('\nnext');
  });

  it('returns empty/null inputs untouched', () => {
    expect(applyCarriageReturns('')).toBe('');
    expect(applyCarriageReturns(undefined)).toBeUndefined();
  });
});

describe('collapseRepeats', () => {
  it('collapses runs of blank lines to a single blank', () => {
    const input = 'top\n\n\n\nbottom';
    expect(collapseRepeats(input)).toBe('top\n\nbottom');
  });

  it('leaves short identical runs (< MIN) verbatim', () => {
    const min = REPEAT_MARKER_MIN;
    expect(min).toBeGreaterThan(2); // sanity: defaults to 4
    const input = 'same\nsame\nsame'; // 3 copies, below threshold
    expect(collapseRepeats(input)).toBe('same\nsame\nsame');
  });

  it('marks long identical runs with the count of additional copies', () => {
    const input = 'same\nsame\nsame\nsame\nsame'; // 5 copies
    expect(collapseRepeats(input)).toBe('same\n  ⟨4× more identical⟩');
  });

  it('separates non-blank runs from each other normally', () => {
    const input = 'a\na\na\na\na\nb\nc';
    expect(collapseRepeats(input)).toBe('a\n  ⟨4× more identical⟩\nb\nc');
  });

  it('does not mark whitespace-only runs even if they are long', () => {
    const input = 'a\n\n\n\n\nb';
    expect(collapseRepeats(input)).toBe('a\n\nb');
  });

  it('handles a transcript ending mid-run', () => {
    const input = 'unique\nrepeat\nrepeat\nrepeat\nrepeat';
    expect(collapseRepeats(input)).toBe('unique\nrepeat\n  ⟨3× more identical⟩');
  });
});

describe('cleanTranscript', () => {
  it('composes the three passes in the right order', () => {
    // ANSI -> CR rewind within line -> blank collapse
    const raw = '\x1B[32mt3 ❯\x1B[0m p\rt3 ❯ pw\rt3 ❯ pwd\n\n\n\nPath\n';
    expect(cleanTranscript(raw)).toBe('t3 ❯ pwd\n\nPath\n');
  });

  it('survives a realistic PowerShell-style snippet', () => {
    const raw = [
      '\x1B]0;PS A:\\repo\x07',
      '\x1B[?25l\x1B[37mPS A:\\repo>\x1B[0m \x1B[?25h',
      'function prompt { "$env:USERNAME ❯ " }; cls',
      '',
      't3 ❯ p\rt3 ❯ pw\rt3 ❯ pwd',
      'Path',
      '----',
      'A:\\repo',
    ].join('\n');
    const cleaned = cleanTranscript(raw);
    expect(cleaned).not.toContain('\x1B');
    expect(cleaned).toContain('t3 ❯ pwd');
    expect(cleaned).not.toContain('p\rpw');
  });

  it('returns empty string for empty input', () => {
    expect(cleanTranscript('')).toBe('');
  });
});
