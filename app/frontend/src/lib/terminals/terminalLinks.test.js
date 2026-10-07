import { describe, expect, test, vi } from 'vitest';
vi.mock('../../../wailsjs/runtime', () => ({ BrowserOpenURL: () => {} }));
import { createHyperlinkHandler, hoveredLink } from './terminalLinks.js';

describe('OSC 8 hyperlinks', () => {
  test('open on Ctrl/Cmd+click only and track the hovered url', () => {
    const opened = [];
    const h = createHyperlinkHandler(42, { open: (u) => opened.push(u) });
    h.activate({ ctrlKey: false }, 'https://git.example/mr/311');
    expect(opened).toEqual([]);
    h.activate({ ctrlKey: true }, 'https://git.example/mr/311');
    h.activate({ metaKey: true }, 'https://git.example/mr/312');
    expect(opened).toEqual(['https://git.example/mr/311', 'https://git.example/mr/312']);
    h.hover({}, 'https://git.example/mr/311');
    expect(hoveredLink(42)).toBe('https://git.example/mr/311');
    h.leave();
    expect(hoveredLink(42)).toBe('');
    expect(h.allowNonHttpProtocols).toBe(false);
  });
});
