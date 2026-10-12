import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { describe, expect, test } from 'vitest';

// lib/backend.js is the only module that may reach window.go.main.App or
// import the generated bindings. This keeps the rule a test failure instead
// of a review comment.
const SRC = join(__dirname, '..');

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name);
    if (statSync(full).isDirectory()) walk(full, out);
    else if (/\.(js|svelte)$/.test(name) && !/\.test\.js$/.test(name)) out.push(full);
  }
  return out;
}

describe('backend facade', () => {
  test('no module outside lib/backend.js talks to the Go side directly', () => {
    const offenders = [];
    for (const file of walk(SRC)) {
      const rel = relative(SRC, file).replace(/\\/g, '/');
      if (rel === 'lib/backend.js') continue;
      const text = readFileSync(file, 'utf8');
      if (/window(\[['"]go['"]\]|\.go\b|\?\.go\b)/.test(text) || /wailsjs\/go\/main\/App/.test(text)) {
        offenders.push(rel);
      }
    }
    expect(offenders).toEqual([]);
  });
});
