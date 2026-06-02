// Lightweight, dependency-free i18n.
//
// Usage in a Svelte component:
//   import { t } from '../../i18n.js';
//   <span>{$t('aiPanel.result')}</span>
//   <span>{$t('aiPanel.reviewNote', { warning })}</span>
//
// Strings live in ./locales/<lang>.js as nested objects; keys are dotted paths.
// English is the default and the fallback for any missing key. Placeholders use
// {name} and are filled from the vars object.
import { writable, derived } from 'svelte/store';
import en from './locales/en.js';
import de from './locales/de.js';

const dictionaries = { en, de };
const STORAGE_KEY = 'mimir-locale';

function detectLocale() {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved && dictionaries[saved]) return saved;
  } catch {
    /* localStorage unavailable */
  }
  return 'en';
}

export const availableLocales = Object.keys(dictionaries);

export const locale = writable(detectLocale());

locale.subscribe((value) => {
  try {
    localStorage.setItem(STORAGE_KEY, value);
  } catch {
    /* ignore */
  }
});

function lookup(dict, key) {
  return key.split('.').reduce((node, part) => (node && node[part] != null ? node[part] : undefined), dict);
}

function fill(template, vars) {
  return String(template).replace(/\{(\w+)\}/g, (_, name) => (vars[name] != null ? vars[name] : `{${name}}`));
}

/**
 * Reactive translation function: `$t(key, vars?)`. Falls back to English, then
 * to the raw key so a missing translation is visible rather than blank.
 */
export const t = derived(locale, ($locale) => {
  const dict = dictionaries[$locale] || dictionaries.en;
  return (key, vars = {}) => {
    let str = lookup(dict, key);
    if (str == null) str = lookup(dictionaries.en, key);
    if (str == null) return key;
    return fill(str, vars);
  };
});
