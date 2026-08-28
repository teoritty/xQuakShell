const STORAGE_KEY = 'xqs.locale';

/**
 * The language code is mirrored outside the vault so the interface can be drawn before the vault is
 * open.
 *
 * Settings live inside vault.age, which means the real setting is unreadable until the user has
 * typed the master password - and the screen asking for it is the one screen a user who cannot read
 * English most needs translated. The vault stays the source of truth; this is a hint used for the
 * first frame and corrected as soon as the settings load.
 *
 * A language code is a public preference, not a secret, which is what makes localStorage the right
 * place for it. Nothing else about the vault is stored here.
 */
export function readStoredLocale(): string | null {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    return stored && stored.trim() ? stored : null;
  } catch {
    // Storage can be unavailable (disabled, or a quota-exhausted profile). English is the answer
    // then, and it is not worth failing startup over a preference.
    return null;
  }
}

export function storeLocale(code: string): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, code);
  } catch {
    // The setting is already saved in the vault; losing the mirror only costs a translated frame
    // on the next unlock screen.
  }
}
