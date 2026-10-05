import { describe, expect, test } from 'vitest';
import { extractSnippets, looksLikeCommand } from './markdownBlocks.js';

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
