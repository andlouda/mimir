import { describe, expect, test } from 'vitest';
import { outputMentionsAgent, parseAgentTitle } from './agentSignals.js';
import { extractCodeBlocks, extractSnippets, firstProse, groupTurns, splitMarkdown } from './markdownBlocks.js';

describe('parseAgentTitle', () => {
  test('recognises Claude Code working and idle titles', () => {
    expect(parseAgentTitle('◐ Bash curl example')).toEqual({ agentLike: true, status: 'working', subject: 'Bash curl example' });
    expect(parseAgentTitle('◑ MIMIR_TEST_TOKEN export')).toMatchObject({ status: 'working' });
    expect(parseAgentTitle('✳ Claude Code')).toEqual({ agentLike: true, status: 'idle', subject: 'Claude Code' });
  });

  test('ignores ordinary shell titles', () => {
    expect(parseAgentTitle('user@host: ~/proj')).toMatchObject({ agentLike: false, status: null });
    expect(parseAgentTitle('')).toMatchObject({ agentLike: false });
    expect(parseAgentTitle(null)).toMatchObject({ agentLike: false });
  });
});

describe('outputMentionsAgent', () => {
  test('matches startup banners only', () => {
    expect(outputMentionsAgent('Welcome to OpenAI Codex v0.137')).toBe(true);
    expect(outputMentionsAgent('✳ Welcome to Claude Code')).toBe(true);
    expect(outputMentionsAgent('ls -la\n')).toBe(false);
    expect(outputMentionsAgent('')).toBe(false);
  });
});

describe('splitMarkdown', () => {
  test('separates prose and fenced code with language', () => {
    const md = 'Run this:\n\n```bash\nexport TOKEN=abc\necho $TOKEN\n```\n\nThen done.';
    const segments = splitMarkdown(md);
    expect(segments).toEqual([
      { type: 'text', text: 'Run this:\n' },
      { type: 'code', lang: 'bash', code: 'export TOKEN=abc\necho $TOKEN' },
      { type: 'text', text: '\nThen done.' },
    ]);
  });

  test('handles tilde fences, nested backticks and unterminated blocks', () => {
    const md = '~~~\n```\ninner\n```\n~~~\n```js\nconst x = 1;';
    const segments = splitMarkdown(md);
    expect(segments[0]).toEqual({ type: 'code', lang: '', code: '```\ninner\n```' });
    expect(segments[1]).toEqual({ type: 'code', lang: 'js', code: 'const x = 1;' });
    expect(extractCodeBlocks(md)).toHaveLength(2);
  });

  test('keeps long code lines intact', () => {
    const line = 'curl -s -H "Authorization: Bearer abc123" "https://example.com/api/v1/items?limit=100&offset=200" | jq ".items[] | .name"';
    const [block] = extractCodeBlocks('```sh\n' + line + '\n```');
    expect(block.code).toBe(line);
  });

  test('plain text without fences is one text segment', () => {
    expect(splitMarkdown('hello')).toEqual([{ type: 'text', text: 'hello' }]);
    expect(splitMarkdown('')).toEqual([]);
  });
});

describe('extractSnippets / firstProse', () => {
  test('collects blocks first, then unique inline code', () => {
    const md = 'Run `go test ./...` then edit `app/main.go`:\n\n```bash\ngo test ./...\n```\n\nAlso `ls` and `app/main.go` again.';
    const snippets = extractSnippets(md);
    expect(snippets).toEqual([
      { type: 'code', lang: 'bash', code: 'go test ./...' },
      { type: 'inline', code: 'app/main.go' },
    ]);
  });

  test('firstProse strips code and caps length', () => {
    const md = '## Result\n\nThe **fix** is in.\n\n```js\nx\n```\n\nDetails follow.';
    expect(firstProse(md)).toBe('Result\nThe fix is in.\nDetails follow.');
    const long = 'word '.repeat(200);
    const summary = firstProse(long, 100);
    expect(summary.length).toBeLessThanOrEqual(101);
    expect(summary.endsWith('…')).toBe(true);
  });
});

describe('groupTurns', () => {
  test('keeps every assistant record of a turn together', () => {
    const messages = [
      { role: 'user', text: 'do it' },
      { role: 'assistant', text: 'Run this:\n\n```bash\nmake test\n```' },
      { role: 'assistant', text: 'Done, tell me if it fails.' },
      { role: 'user', text: 'thanks' },
      { role: 'assistant', text: 'Welcome.' },
    ];
    const turns = groupTurns(messages);
    expect(turns).toHaveLength(2);
    expect(turns[0].prompt.text).toBe('do it');
    expect(turns[0].answers.map((a) => a.text)).toEqual(['Run this:\n\n```bash\nmake test\n```', 'Done, tell me if it fails.']);
    expect(turns[1].answers).toHaveLength(1);
    // Turns without an answer are dropped; leading answers without a prompt are kept.
    expect(groupTurns([{ role: 'user', text: 'x' }])).toEqual([]);
    expect(groupTurns([{ role: 'assistant', text: 'hi' }])[0].prompt).toBeNull();
  });
});
