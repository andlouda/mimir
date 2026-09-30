import { describe, expect, test } from 'vitest';
import { get } from 'svelte/store';
import { TERMINAL_FONT_DEFAULT, TERMINAL_FONT_MAX, TERMINAL_FONT_MIN, clampFontSize, resetTerminalFontSize, setTerminalFontSize, terminalFontSize, zoomTerminalFont } from './uiStore.js';
import { isGlobalShortcut, isZoomShortcut } from '../actions/keyboardShortcuts.js';

describe('terminal zoom', () => {
  test('font size is clamped and zoom steps stay in range', () => {
    expect(clampFontSize('abc')).toBe(TERMINAL_FONT_DEFAULT);
    expect(clampFontSize(2)).toBe(TERMINAL_FONT_MIN);
    expect(clampFontSize(99)).toBe(TERMINAL_FONT_MAX);
    setTerminalFontSize(TERMINAL_FONT_MAX);
    zoomTerminalFont(1);
    expect(get(terminalFontSize)).toBe(TERMINAL_FONT_MAX);
    zoomTerminalFont(-3);
    expect(get(terminalFontSize)).toBe(TERMINAL_FONT_MAX - 3);
    resetTerminalFontSize();
    expect(get(terminalFontSize)).toBe(TERMINAL_FONT_DEFAULT);
  });

  test('Ctrl +/-/0 are global shortcuts, plain keys are not', () => {
    for (const key of ['+', '=', '-', '0']) {
      expect(isZoomShortcut({ ctrlKey: true, key })).toBe(true);
      expect(isGlobalShortcut({ ctrlKey: true, key })).toBe(true);
    }
    expect(isZoomShortcut({ ctrlKey: false, key: '+' })).toBe(false);
    expect(isZoomShortcut({ ctrlKey: true, altKey: true, key: '+' })).toBe(false);
    expect(isGlobalShortcut({ ctrlKey: true, key: 'c' })).toBe(false);
  });
});
