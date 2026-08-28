import { derived, get, writable } from 'svelte/store';
import { interpolate, pluralKey, type MessageVars } from './format';

/** The language currently displayed. Read it when a format depends on the locale, not on a string. */
export const currentLocale = writable('en');

/** The message catalogue for the current language, already merged with English by the backend. */
export const messages = writable<Record<string, string>>({});

/**
 * The translation function, as a store so a template re-renders when the language changes.
 *
 * Usage is `{$t('settings.tab.files')}`, or `{$t('files.selected', { count })}` when the string
 * takes variables. A `count` variable also selects the plural form.
 *
 * A key with no translation renders its own key rather than an empty string. The backend has
 * already laid English under every language, so reaching this fallback means the key exists in no
 * pack at all - a missing string in the source, which should be visible rather than swallowed.
 */
export const t = derived([messages, currentLocale], ([$messages, $locale]) => {
  return (key: string, vars?: MessageVars): string => {
    const resolved =
      typeof vars?.count === 'number' ? pluralKey($messages, key, $locale, vars.count) : key;
    const template = $messages[resolved];
    if (template === undefined) return key;
    return interpolate(template, vars);
  };
});

/**
 * Translates outside a template, where a store subscription is not available - an error label
 * assembled in a `.ts` module, for instance.
 *
 * Prefer `$t` in components: this reads the catalogue once, so a string produced here does not
 * update when the language changes.
 */
export function translate(key: string, vars?: MessageVars): string {
  return get(t)(key, vars);
}
