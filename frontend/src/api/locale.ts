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

/**
 * The language a secondary window was launched in, or null in the main window, which has no such
 * thing and reads the setting instead.
 *
 * The log viewer is a separate process: the vault it would read the language from is never
 * unlocked there, and the localStorage mirror belongs to the main window's WebView, so the parent
 * passes the code on the command line and the window asks for it here.
 */
export async function fetchLaunchLocale(): Promise<string | null> {
  return callBackend('Load window language', null, async (app) => {
    if (!app.LaunchLocale) return null;
    return (await app.LaunchLocale()) || null;
  });
}
