// @vitest-environment jsdom
import { describe, expect, test } from 'vitest';
import { containsControlChars, sanitizeHtml, shellQuotePath } from './util.js';

describe('sanitizeHtml', () => {
  test('keeps the Markdown subset', () => {
    const out = sanitizeHtml('<h1>T</h1><p>a <strong>b</strong> <code>c</code></p><table><tr><td>x</td></tr></table>');
    expect(out).toContain('<h1>T</h1>');
    expect(out).toContain('<strong>b</strong>');
    expect(out).toContain('<td>x</td>');
  });

  test('strips script, event handlers and javascript: URLs', () => {
    const out = sanitizeHtml('<p onclick="alert(1)">x</p><script>alert(1)</script><a href="javascript:alert(1)">l</a><img src="x" onerror="alert(1)">');
    expect(out).not.toContain('script');
    expect(out).not.toContain('onclick');
    expect(out).not.toContain('onerror');
    expect(out).not.toContain('javascript:');
  });

  test('survives classic mXSS / foreign-content payloads', () => {
    const payloads = [
      '<svg><style><img src=x onerror=alert(1)></style></svg>',
      '<math><mtext><table><mglyph><style><img src=x onerror=alert(1)></style></mglyph></table></mtext></math>',
      '<table><a href="javascript:alert(1)">x</a></table>',
      '<noscript><p title="</noscript><img src=x onerror=alert(1)>">',
      '<form><math><mtext></form><form><mglyph><style></math><img src onerror=alert(1)>',
      '<a href="\u0001javascript:alert(1)">x</a>',
    ];
    for (const payload of payloads) {
      const out = sanitizeHtml(payload);
      expect(out, payload).not.toMatch(/onerror|javascript:|<script|<svg|<math|<style|<form/i);
    }
  });

  test('blocks remote images but keeps inline data images', () => {
    const remote = sanitizeHtml('<img src="https://tracker.example/pixel.png" alt="p">');
    expect(remote).not.toContain('https://tracker.example');
    const inline = sanitizeHtml('<img src="data:image/png;base64,iVBORw0KGgo=" alt="p">');
    expect(inline).toContain('data:image/png;base64,iVBORw0KGgo=');
    const dataHtml = sanitizeHtml('<a href="data:text/html;base64,PHNjcmlwdD4=">x</a>');
    expect(dataHtml).not.toContain('data:text/html');
  });

  test('forces safe rel on links and drops non-_blank targets', () => {
    const out = sanitizeHtml('<a href="https://example.com" target="_top">x</a><a href="https://example.com" target="_blank">y</a>');
    expect(out).toContain('rel="noreferrer noopener"');
    expect(out).not.toContain('_top');
    expect(out).toContain('target="_blank"');
  });

  test('strips attributes that are only meaningful on other tags', () => {
    const out = sanitizeHtml('<p href="https://x" src="https://y" target="_blank">t</p>');
    expect(out).toBe('<p>t</p>');
  });
});

describe('shell path helpers', () => {
  test('containsControlChars detects CR, ESC and DEL', () => {
    expect(containsControlChars('/home/u/proj')).toBe(false);
    expect(containsControlChars("/tmp/it's")).toBe(false);
    expect(containsControlChars('/tmp/a\rrm -rf ~')).toBe(true);
    expect(containsControlChars('/tmp/\x1b[A')).toBe(true);
    expect(containsControlChars('/tmp/\x7f')).toBe(true);
    expect(containsControlChars(null)).toBe(false);
  });

  test('shellQuotePath escapes single quotes', () => {
    expect(shellQuotePath("/tmp/it's")).toBe("'/tmp/it'\\''s'");
  });
});
