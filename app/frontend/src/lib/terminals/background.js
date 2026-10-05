// Per-pane background image settings.
//
// A pane's background is { id, opacity, blur, fit }: the imported image's
// id (served by the backend at /mimir-bg/<id>), how visible the picture is
// over the dark pane colour, an optional blur and how it fills the pane.
// Stored with the pane in the session as JSON (see app_backgrounds.go).

export const BACKGROUND_FITS = ['cover', 'contain', 'tile', 'center'];
export const BACKGROUND_DEFAULTS = Object.freeze({ opacity: 0.35, blur: 0, fit: 'cover' });

const ID_PATTERN = /^[0-9a-f]{16}\.(png|jpg|webp|gif)$/;

export function isBackgroundId(id) {
  return typeof id === 'string' && ID_PATTERN.test(id);
}

function clamp(n, min, max, fallback) {
  const v = Number(n);
  if (!Number.isFinite(v)) return fallback;
  return Math.min(max, Math.max(min, v));
}

/** Returns a complete, validated setting or null when there is no image. */
export function normalizeBackground(raw) {
  if (!raw || !isBackgroundId(raw.id)) return null;
  return {
    id: raw.id,
    opacity: clamp(raw.opacity, 0.05, 1, BACKGROUND_DEFAULTS.opacity),
    blur: Math.round(clamp(raw.blur, 0, 24, BACKGROUND_DEFAULTS.blur)),
    fit: BACKGROUND_FITS.includes(raw.fit) ? raw.fit : BACKGROUND_DEFAULTS.fit,
  };
}

/** Parses the JSON stored in the session; invalid or empty → null. */
export function parseBackground(json) {
  if (!json) return null;
  try {
    return normalizeBackground(JSON.parse(json));
  } catch {
    return null;
  }
}

export function serializeBackground(bg) {
  const n = normalizeBackground(bg);
  return n ? JSON.stringify(n) : '';
}

export function backgroundUrl(id) {
  return isBackgroundId(id) ? `/mimir-bg/${id}` : '';
}

/** Inline style of the image layer behind the terminal. */
export function backgroundLayerStyle(bg) {
  const n = normalizeBackground(bg);
  if (!n) return '';
  const parts = [`background-image: url(${backgroundUrl(n.id)})`, `opacity: ${n.opacity}`];
  switch (n.fit) {
    case 'contain':
      parts.push('background-size: contain', 'background-repeat: no-repeat', 'background-position: center');
      break;
    case 'tile':
      parts.push('background-size: auto', 'background-repeat: repeat', 'background-position: top left');
      break;
    case 'center':
      parts.push('background-size: auto', 'background-repeat: no-repeat', 'background-position: center');
      break;
    default:
      parts.push('background-size: cover', 'background-repeat: no-repeat', 'background-position: center');
  }
  // Blur bleeds at the edges; the layer is slightly oversized to hide that.
  if (n.blur > 0) parts.push(`filter: blur(${n.blur}px)`, `inset: -${n.blur * 2}px`);
  return parts.join('; ') + ';';
}
