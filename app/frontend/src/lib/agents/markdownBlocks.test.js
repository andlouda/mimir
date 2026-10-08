import { describe, expect, test } from 'vitest';
import { extractSnippets, looksLikeCommand, extractLinks, plainCommandLines, splitMarkdown } from './markdownBlocks.js';

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

  test('env prefixes, prompts and PowerShell count; label: value pairs do not', () => {
    expect(looksLikeCommand('GOFLAGS=-mod=mod go build ./...')).toBe(true);
    expect(looksLikeCommand('$ make deploy')).toBe(true);
    expect(looksLikeCommand('Get-ChildItem -Recurse -Filter *.log')).toBe(true);
    expect(looksLikeCommand('.\\build.ps1 -Clean')).toBe(true);
    expect(looksLikeCommand('deploy stack: ai-agents')).toBe(false);
    expect(looksLikeCommand('Status: done')).toBe(false);
  });

  test('plain-text command lines: prompt prefix, under a label, label on one line', () => {
    const md = [
      'Alles gebaut. Zum Starten:',
      '  pants run deploy:ai-agents',
      '  docker compose up -d',
      '',
      'Danach $ git push origin main',
      '$ go test ./...',
      'Deploy: kubectl apply -f k8s/',
      'Hinweis: das dauert ein paar Minuten',
      'Dann:',
      'Die Tests laufen jetzt durch.',
      'Kurz:',
      'tmux ist jetzt die Basis, die Dateien nur noch Zusatz.',
    ].join('\n');
    expect(plainCommandLines(md)).toEqual([
      'pants run deploy:ai-agents',
      'docker compose up -d',
      'go test ./...',
      'kubectl apply -f k8s/',
    ]);
  });

  test('plain lines are deduped against inline and block snippets', () => {
    const md = 'Run:\n  npm test\n\nor `npm test` again.\n\n```sh\nnpm run build\n```\n$ npm run build\n';
    expect(extractSnippets(md).map((s) => `${s.type}:${s.code.trim()}`)).toEqual(['code:npm run build', 'inline:npm test']);
  });

  test('fenced blocks nested deep in lists are still blocks', () => {
    const md = '1. Install\n   - then run:\n\n      ```bash\n      npm i\n      ```\n';
    expect(splitMarkdown(md).filter((s) => s.type === 'code')).toEqual([{ type: 'code', lang: 'bash', code: 'npm i' }]);
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
