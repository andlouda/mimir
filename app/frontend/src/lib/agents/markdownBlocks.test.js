import { describe, expect, test } from 'vitest';
import { extractSnippets, looksLikeCommand, extractLinks } from './markdownBlocks.js';

describe('inline snippets', () => {
  test('only command-shaped inline code becomes a snippet', () => {
    expect(looksLikeCommand('go test ./...')).toBe(true);
    expect(looksLikeCommand('npm run build')).toBe(true);
    expect(looksLikeCommand('./scripts/run.sh --fast')).toBe(true);
    expect(looksLikeCommand('TeamSpeak.exe')).toBe(false);
    expect(looksLikeCommand('--type=utility')).toBe(false);
    expect(looksLikeCommand('network.mojom.NetworkService')).toBe(false);
    expect(looksLikeCommand('App network helper')).toBe(false);
    expect(looksLikeCommand('cli.js --flag')).toBe(false);
  });

  test('fenced blocks always count, inline names do not', () => {
    const md = 'Run `go test ./...` and look at `TeamSpeak.exe` with `--lang=de`.\n\n```sh\nnpm run build\n```\n';
    const snippets = extractSnippets(md);
    expect(snippets.map((s) => `${s.type}:${s.code.trim()}`)).toEqual(['code:npm run build', 'inline:go test ./...']);
  });
});

describe('links', () => {
  test('markdown links and bare URLs in order, deduped, punctuation stripped', () => {
    const md = 'See [MR !311](https://gitlab.example.com/g/p/-/merge_requests/311) and https://gitlab.example.com/g/p/-/merge_requests/311.\n' +
      'Docs: https://example.com/docs/page, also `curl https://api.example.com/v1`';
    expect(extractLinks(md)).toEqual([
      { type: 'link', url: 'https://gitlab.example.com/g/p/-/merge_requests/311', label: 'MR !311' },
      { type: 'link', url: 'https://example.com/docs/page', label: '' },
      { type: 'link', url: 'https://api.example.com/v1', label: '' },
    ]);
    expect(extractLinks('no links here')).toEqual([]);
  });
});
