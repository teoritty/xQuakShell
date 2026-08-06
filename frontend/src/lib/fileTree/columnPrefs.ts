// Which optional columns a file pane shows, and remembering that across restarts.
//
// Both panes read the same JSON shape out of localStorage under their own key,
// with the same try/catch around it, and both wrote it back from three
// near-identical toggle functions. Only the storage keys differ. The parsing is
// the part worth having a test for: what is in localStorage is user-editable
// text that has already been through older versions of this app, so it has to
// survive being absent, empty, malformed, or the wrong type.

export interface ColumnPrefs {
  permissions: boolean;
  owner: boolean;
  date: boolean;
}

export interface PaneStorageKeys {
  columns: string;
  hidden: string;
}

/** Minimal Storage surface, so the parsing is testable without a browser. */
export interface PrefsStore {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

export const NO_COLUMNS: ColumnPrefs = { permissions: false, owner: false, date: false };

/** Stored column prefs, or all-off for anything that cannot be read as prefs. */
export function parseColumnPrefs(stored: string | null): ColumnPrefs {
  if (!stored) return { ...NO_COLUMNS };
  try {
    const parsed = JSON.parse(stored);
    if (!parsed || typeof parsed !== 'object') return { ...NO_COLUMNS };
    return {
      permissions: !!parsed.permissions,
      owner: !!parsed.owner,
      date: !!parsed.date,
    };
  } catch {
    return { ...NO_COLUMNS };
  }
}

/**
 * Load a pane's saved view preferences.
 *
 * A storage that throws on read — Safari in private mode, a locked-down
 * embedder — is not an error worth surfacing: the pane opens with its columns
 * off, which is exactly what a first-ever launch does.
 */
export function loadPrefs(store: PrefsStore, keys: PaneStorageKeys): { columns: ColumnPrefs; showHidden: boolean } {
  try {
    return {
      columns: parseColumnPrefs(store.getItem(keys.columns)),
      showHidden: store.getItem(keys.hidden) === '1',
    };
  } catch {
    return { columns: { ...NO_COLUMNS }, showHidden: false };
  }
}

/** Persist column prefs, ignoring a storage that refuses to be written. */
export function saveColumnPrefs(store: PrefsStore, keys: PaneStorageKeys, columns: ColumnPrefs): void {
  try {
    store.setItem(keys.columns, JSON.stringify(columns));
  } catch {
    /* preferences are a convenience; failing to save one must not break the pane */
  }
}

/** Persist the show-hidden toggle, ignoring a storage that refuses to be written. */
export function saveHiddenPref(store: PrefsStore, keys: PaneStorageKeys, showHidden: boolean): void {
  try {
    store.setItem(keys.hidden, showHidden ? '1' : '0');
  } catch {
    /* see saveColumnPrefs */
  }
}
