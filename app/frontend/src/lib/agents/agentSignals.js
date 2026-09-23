// Pure helpers that turn terminal side channels (window title, output text)
// into agent signals. Kept free of stores so they are trivially testable.

// Claude Code animates its window title while working: a spinner glyph is
// followed by a short subject ("◐ Bash curl example"). When it is idle the
// glyph becomes ✳ ("✳ Claude Code" right after start).
const WORKING_GLYPHS = new Set(['◐', '◑', '◒', '◓', '◴', '◵', '◶', '◷']);
const IDLE_GLYPHS = new Set(['✳', '✻', '✶', '✽', '✢', '*', '·']);

/**
 * Parses a terminal title into an agent status hint.
 * @returns {{ agentLike: boolean, status: 'working'|'idle'|null, subject: string }}
 */
export function parseAgentTitle(title) {
  const text = String(title ?? '').trim();
  if (!text) return { agentLike: false, status: null, subject: '' };
  const glyph = Array.from(text)[0];
  const subject = text.slice(glyph.length).trim();
  if (WORKING_GLYPHS.has(glyph)) return { agentLike: true, status: 'working', subject };
  if (IDLE_GLYPHS.has(glyph)) return { agentLike: true, status: 'idle', subject };
  return { agentLike: false, status: null, subject: text };
}

// Banner fragments the supported agents print on startup. Seeing one in the
// output stream is a cheap trigger to (re)run process detection.
const BANNER_PATTERN = /OpenAI Codex|Claude Code|Gemini CLI|opencode|Aider v\d|Hermes/i;

/** True when a chunk of terminal output looks like an agent starting up. */
export function outputMentionsAgent(chunk) {
  if (!chunk || chunk.length > 200000) return false;
  return BANNER_PATTERN.test(chunk);
}
