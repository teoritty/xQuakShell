// Atomic locale RPC wrappers. Both calls reach the backend's language catalogue, which reads the
// packs compiled into the binary plus any the user dropped into <exe>/data/locales. Neither touches
// the vault, so both work before it is unlocked. No store access here.
import { callBackend } from '../backend/callBackend';

export interface LocaleInfo {
  code: string;
  name: string;
  builtin: boolean;
}

export interface LocaleMessages {
  code: string;
  name: string;
  messages: Record<string, string>;
}

/** Lists the selectable languages, or an empty list when the backend has no catalogue. */
export async function listLocales(): Promise<LocaleInfo[]> {
  return callBackend('List languages', [] as LocaleInfo[], async (app) => {
    if (!app.ListLocales) return [];
    return ((await app.ListLocales()) ?? []) as LocaleInfo[];
  });
}

/**
 * Fetches one language's catalogue. The English fallback is applied by the backend, so what comes
 * back is complete and the caller needs no fallback of its own.
 */
export async function fetchLocaleMessages(code: string): Promise<LocaleMessages | null> {
  return callBackend('Load language', null, async (app) => {
    if (!app.GetLocaleMessages) return null;
    return (await app.GetLocaleMessages(code)) as LocaleMessages;
  });
}
