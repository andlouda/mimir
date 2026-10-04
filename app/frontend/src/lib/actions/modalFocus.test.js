// @vitest-environment jsdom
import { describe, expect, test, vi } from 'vitest';
import { modalFocus, pickInitialFocus } from './modalFocus.js';

function dialog() {
  document.body.innerHTML = `
    <button id="trigger">open</button>
    <div id="dlg" role="dialog">
      <button class="modal-close-button" id="x">✕</button>
      <input id="name" />
      <button id="ok">OK</button>
    </div>`;
  return document.getElementById('dlg');
}

describe('modalFocus', () => {
  test('moves focus to the first text field and restores the trigger on destroy', () => {
    vi.useFakeTimers();
    const dlg = dialog();
    document.getElementById('trigger').focus();
    const action = modalFocus(dlg, { onEscape: vi.fn() });
    vi.runAllTimers();
    expect(document.activeElement.id).toBe('name');
    action.destroy();
    expect(document.activeElement.id).toBe('trigger');
    vi.useRealTimers();
  });

  test('traps Tab inside and closes on Escape', () => {
    vi.useFakeTimers();
    const dlg = dialog();
    const onEscape = vi.fn();
    const action = modalFocus(dlg, { onEscape });
    vi.runAllTimers();
    document.getElementById('ok').focus();
    dlg.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }));
    expect(document.activeElement.id).toBe('x');
    dlg.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true }));
    expect(document.activeElement.id).toBe('ok');
    document.getElementById('name').dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    expect(onEscape).toHaveBeenCalledTimes(1);
    action.destroy();
    vi.useRealTimers();
  });

  test('prefers a text field over the close button', () => {
    const dlg = dialog();
    expect(pickInitialFocus(dlg).id).toBe('name');
    document.getElementById('name').remove();
    expect(pickInitialFocus(dlg).id).toBe('ok');
  });
});
