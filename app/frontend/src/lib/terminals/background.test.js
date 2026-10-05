import { describe, expect, test } from 'vitest';
import { backgroundLayerStyle, normalizeBackground, parseBackground, serializeBackground } from './background.js';

const ID = '0123456789abcdef.png';

describe('pane background settings', () => {
  test('normalizes and clamps', () => {
    expect(normalizeBackground(null)).toBeNull();
    expect(normalizeBackground({ id: '../etc/passwd' })).toBeNull();
    expect(normalizeBackground({ id: ID })).toEqual({ id: ID, opacity: 0.35, blur: 0, fit: 'cover' });
    expect(normalizeBackground({ id: ID, opacity: 7, blur: -3, fit: 'stretch' })).toEqual({ id: ID, opacity: 1, blur: 0, fit: 'cover' });
    expect(normalizeBackground({ id: ID, opacity: '0.5', blur: '4.4', fit: 'tile' })).toEqual({ id: ID, opacity: 0.5, blur: 4, fit: 'tile' });
  });

  test('round-trips through the session JSON', () => {
    const json = serializeBackground({ id: ID, opacity: 0.6, blur: 2, fit: 'contain' });
    expect(parseBackground(json)).toEqual({ id: ID, opacity: 0.6, blur: 2, fit: 'contain' });
    expect(serializeBackground(null)).toBe('');
    expect(parseBackground('{not json')).toBeNull();
    expect(parseBackground('')).toBeNull();
  });

  test('builds the layer style', () => {
    const style = backgroundLayerStyle({ id: ID, opacity: 0.4, blur: 6, fit: 'tile' });
    expect(style).toContain(`url(/mimir-bg/${ID})`);
    expect(style).toContain('opacity: 0.4');
    expect(style).toContain('background-repeat: repeat');
    expect(style).toContain('filter: blur(6px)');
    expect(backgroundLayerStyle(null)).toBe('');
  });
});
