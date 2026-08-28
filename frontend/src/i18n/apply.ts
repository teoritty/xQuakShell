import { fetchLocaleMessages } from '../api/locale';
import { currentLocale, messages } from './messages';
import { readStoredLocale, storeLocale } from './persist';

export const DEFAULT_LOCALE = 'en';

/**
 * Switches the interface to a language and remembers the choice outside the vault.
 *
 * The catalogue arrives complete - the backend has already laid English under it - so this either
 * replaces the whole catalogue or leaves the previous one standing. A half-applied language would
 * show two languages at once, which is worse than showing the old one.
 */
export async function applyLocale(code: string): Promise<void> {
  const pack = await fetchLocaleMessages(code);
  if (!pack) return;
  messages.set(pack.messages ?? {});
  currentLocale.set(pack.code || DEFAULT_LOCALE);
  storeLocale(pack.code || DEFAULT_LOCALE);
  document.documentElement.lang = pack.code || DEFAULT_LOCALE;
}

/**
 * Loads the language to draw the first frame in, before anything has unlocked the vault.
 *
 * This is what makes the master password screen appear in the user's language rather than in
 * English; the real setting replaces it as soon as the vault opens.
 */
export async function initLocale(): Promise<void> {
  await applyLocale(readStoredLocale() ?? DEFAULT_LOCALE);
}
