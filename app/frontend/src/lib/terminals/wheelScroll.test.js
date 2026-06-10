import { describe, expect, test } from 'vitest';
import { consumeWheelEvent, resetWheelRemainder, wheelEventToLines } from './wheelScroll.js';

const LINE_HEIGHT = 18;
const ROWS = 40;

function pixelEvent(deltaY) {
  return { deltaY, deltaMode: 0 };
}

describe('wheelEventToLines', () => {
  test('converts pixel deltas using the line height', () => {
    expect(wheelEventToLines(pixelEvent(36), LINE_HEIGHT, ROWS)).toBe(2);
  });

  test('passes line deltas through unchanged', () => {
    expect(wheelEventToLines({ deltaY: 3, deltaMode: 1 }, LINE_HEIGHT, ROWS)).toBe(3);
  });

  test('scales page deltas by the row count', () => {
    expect(wheelEventToLines({ deltaY: 1, deltaMode: 2 }, LINE_HEIGHT, ROWS)).toBe(ROWS);
  });

  test('guards against a zero line height', () => {
    expect(wheelEventToLines(pixelEvent(5), 0, ROWS)).toBe(5);
  });
});

describe('consumeWheelEvent', () => {
  test('accumulates small touchpad deltas until a whole line is reached', () => {
    const key = {};
    expect(consumeWheelEvent(key, pixelEvent(4.5), LINE_HEIGHT, ROWS)).toBe(0);
    expect(consumeWheelEvent(key, pixelEvent(4.5), LINE_HEIGHT, ROWS)).toBe(0);
    expect(consumeWheelEvent(key, pixelEvent(4.5), LINE_HEIGHT, ROWS)).toBe(0);
    expect(consumeWheelEvent(key, pixelEvent(4.5), LINE_HEIGHT, ROWS)).toBe(1);
  });

  test('keeps the fractional remainder across large scrolls', () => {
    const key = {};
    expect(consumeWheelEvent(key, pixelEvent(45), LINE_HEIGHT, ROWS)).toBe(2);
    // 45px = 2.5 lines -> remainder 0.5; another 9px (0.5 lines) completes a line.
    expect(consumeWheelEvent(key, pixelEvent(9), LINE_HEIGHT, ROWS)).toBe(1);
  });

  test('accumulates negative (scroll up) deltas symmetrically', () => {
    const key = {};
    expect(consumeWheelEvent(key, pixelEvent(-9), LINE_HEIGHT, ROWS)).toBe(0);
    expect(consumeWheelEvent(key, pixelEvent(-9), LINE_HEIGHT, ROWS)).toBe(-1);
  });

  test('direction change does not get stuck on stale remainders', () => {
    const key = {};
    consumeWheelEvent(key, pixelEvent(9), LINE_HEIGHT, ROWS);
    expect(consumeWheelEvent(key, pixelEvent(-18), LINE_HEIGHT, ROWS)).toBe(0);
    expect(consumeWheelEvent(key, pixelEvent(-9), LINE_HEIGHT, ROWS)).toBe(-1);
  });

  test('tracks remainders independently per terminal', () => {
    const a = {};
    const b = {};
    consumeWheelEvent(a, pixelEvent(9), LINE_HEIGHT, ROWS);
    expect(consumeWheelEvent(b, pixelEvent(9), LINE_HEIGHT, ROWS)).toBe(0);
    expect(consumeWheelEvent(a, pixelEvent(9), LINE_HEIGHT, ROWS)).toBe(1);
  });

  test('resetWheelRemainder clears accumulated state', () => {
    const key = {};
    consumeWheelEvent(key, pixelEvent(17), LINE_HEIGHT, ROWS);
    resetWheelRemainder(key);
    expect(consumeWheelEvent(key, pixelEvent(17), LINE_HEIGHT, ROWS)).toBe(0);
  });
});
