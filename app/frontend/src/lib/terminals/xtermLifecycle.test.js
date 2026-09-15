import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { observeTerminalResize, rebindTerminalResize } from './xtermLifecycle.js';

class FakeResizeObserver {
  constructor(cb) { this.cb = cb; this.targets = []; FakeResizeObserver.instances.push(this); }
  observe(el) { this.targets.push(el); }
  disconnect() { this.targets = []; }
}
FakeResizeObserver.instances = [];

beforeEach(() => {
  FakeResizeObserver.instances = [];
  global.ResizeObserver = FakeResizeObserver;
  global.requestAnimationFrame = (cb) => { cb(); return 1; };
  global.cancelAnimationFrame = () => {};
});

afterEach(() => {
  delete global.ResizeObserver;
  delete global.requestAnimationFrame;
  delete global.cancelAnimationFrame;
});

function fakeTerm(id, parent) {
  const element = { parentElement: parent };
  return {
    id,
    minimized: false,
    terminal: { element, rows: 24, cols: 80 },
    fitAddon: { fit: vi.fn() },
  };
}

describe('observeTerminalResize', () => {
  test('watches xterm root and its container, and follows a new container on rebind', () => {
    const containerA = { name: 'A' };
    const term = fakeTerm(1, containerA);
    const resize = vi.fn();
    const dispose = observeTerminalResize(term, resize);
    const [observer] = FakeResizeObserver.instances;
    expect(observer.targets).toEqual([term.terminal.element, containerA]);

    // Layout change: Svelte re-created the container and xterm was moved.
    const containerB = { name: 'B' };
    term.terminal.element.parentElement = containerB;
    rebindTerminalResize(term);
    expect(observer.targets).toEqual([term.terminal.element, containerB]);

    // A size change refits and reports rows/cols to the backend.
    observer.cb();
    expect(term.fitAddon.fit).toHaveBeenCalled();
    expect(resize).toHaveBeenCalledWith(1, '24', '80');

    dispose();
    rebindTerminalResize(term);
    expect(observer.targets).toEqual([]);
  });

  test('no-op without an element', () => {
    const dispose = observeTerminalResize({ id: 2, terminal: {} }, vi.fn());
    expect(FakeResizeObserver.instances).toHaveLength(0);
    dispose();
  });
});
