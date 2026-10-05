import { describe, expect, test } from 'vitest';
import { droppedPathsText, isFileDrop, pathsFromUriList, quotePathFor, shellQuotePath, toWslPath } from './droppedPaths.js';

describe('dropped paths', () => {
  test('detects file drops by dataTransfer types', () => {
    expect(isFileDrop({ types: ['Files'] })).toBe(true);
    expect(isFileDrop({ types: ['text/uri-list', 'text/plain'] })).toBe(true);
    expect(isFileDrop({ types: ['text/plain'] })).toBe(false);
    expect(isFileDrop(null)).toBe(false);
  });

  test('converts file URIs to paths on Linux and Windows', () => {
    expect(pathsFromUriList('file:///home/u/My%20Docs/a.txt\r\n# comment\nfile:///C:/Users/t3/x.log\n')).toEqual(['/home/u/My Docs/a.txt', 'C:/Users/t3/x.log']);
    expect(pathsFromUriList('https://example.com/x')).toEqual([]);
  });

  test('quotes paths for the shell and joins them without a newline', () => {
    expect(shellQuotePath('/plain/path-1.txt')).toBe('/plain/path-1.txt');
    expect(shellQuotePath("/My Docs/it's.txt")).toBe("'/My Docs/it'\\''s.txt'");
    const text = droppedPathsText('file:///a/b.txt\nfile:///c%20d/e.txt');
    expect(text).toBe("/a/b.txt '/c d/e.txt'");
    expect(text.includes('\n')).toBe(false);
  });

  test('quotes per pane type: cmd, PowerShell, WSL', () => {
    expect(quotePathFor('C:/Users/Andre Louda/a.txt', 'cmd')).toBe('"C:/Users/Andre Louda/a.txt"');
    expect(quotePathFor('C:/plain/a.txt', 'cmd')).toBe('C:/plain/a.txt');
    expect(quotePathFor("C:/Users/it's/a.txt", 'powershell')).toBe("'C:/Users/it''s/a.txt'");
    expect(toWslPath('C:/Users/Andre Louda/a.txt')).toBe('/mnt/c/Users/Andre Louda/a.txt');
    expect(toWslPath('D:\\data\\x.log')).toBe('/mnt/d/data/x.log');
    expect(toWslPath('/already/posix')).toBe('/already/posix');
    expect(quotePathFor('C:/Users/Andre Louda/a.txt', 'wsl')).toBe("'/mnt/c/Users/Andre Louda/a.txt'");
    expect(droppedPathsText('file:///C:/Users/Andre%20Louda/a.txt', 'cmd')).toBe('"C:/Users/Andre Louda/a.txt"');
  });
});
