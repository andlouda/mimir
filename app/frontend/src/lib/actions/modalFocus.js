// Svelte action for dialogs: moves focus into the dialog when it opens
// (first text field, else the first control that is not the close "✕"),
// keeps Tab inside, closes on Escape through `onEscape`, and gives focus
// back to whoever opened the dialog when it goes away (usually the active
// terminal's textarea). Lifted from TranscriptViewerModal, which was the
// only modal doing this; every other modal left focus in the terminal, so
// keystrokes went to the shell and Escape did nothing.
//
//   <div role="dialog" use:modalFocus={{ onEscape: onClose }}>…</div>

const FOCUSABLE = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',');

export function focusableIn(node) {
  return Array.from(node.querySelectorAll(FOCUSABLE)).filter((el) =>
    !el.hasAttribute('disabled') && !el.closest('[aria-hidden="true"]') && isVisible(el)
  );
}

function isVisible(el) {
  // jsdom has no layout (offsetParent is always null there); only hide
  // elements that are really display:none in a real browser.
  if (typeof el.getClientRects !== 'function') return true;
  if (el.getClientRects().length > 0) return true;
  return typeof navigator !== 'undefined' && /jsdom/i.test(navigator.userAgent || '');
}

export function pickInitialFocus(node) {
  const list = focusableIn(node);
  return list.find((el) => el.matches('input, textarea, select'))
    || list.find((el) => !el.classList.contains('modal-close-button'))
    || list[0]
    || node;
}

export function modalFocus(node, params = {}) {
  let opts = { ...params };
  const trigger = typeof document !== 'undefined' && document.activeElement instanceof HTMLElement ? document.activeElement : null;
  if (!node.hasAttribute('tabindex')) node.setAttribute('tabindex', '-1');

  const onKeydown = (e) => {
    if (e.key === 'Escape') {
      if (typeof opts.onEscape !== 'function') return;
      e.preventDefault();
      e.stopPropagation();
      opts.onEscape(e);
      return;
    }
    if (e.key !== 'Tab') return;
    const list = focusableIn(node);
    if (list.length === 0) {
      e.preventDefault();
      node.focus();
      return;
    }
    const first = list[0];
    const last = list[list.length - 1];
    const active = document.activeElement;
    if (e.shiftKey && (active === first || active === node || !node.contains(active))) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && (active === last || !node.contains(active))) {
      e.preventDefault();
      first.focus();
    }
  };
  node.addEventListener('keydown', onKeydown);

  let timer = setTimeout(() => {
    timer = null;
    if (opts.initialFocus === false) return;
    try { pickInitialFocus(node).focus({ preventScroll: true }); } catch { /* ignore */ }
  }, 0);

  return {
    update(next) { opts = { ...next }; },
    destroy() {
      if (timer) clearTimeout(timer);
      node.removeEventListener('keydown', onKeydown);
      if (opts.restoreFocus === false) return;
      if (trigger && typeof trigger.focus === 'function' && document.contains(trigger)) {
        try { trigger.focus({ preventScroll: true }); } catch { /* ignore */ }
      }
    },
  };
}
