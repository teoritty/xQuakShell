export type SettingsTabId =
  | 'about'
  | 'appearance'
  | 'audit'
  | 'files'
  | 'hotkeys'
  | 'network'
  | 'security';

export interface SettingsSectionIndex {
  tabId: SettingsTabId;
  sectionId: string;
}

/**
 * The words a section answers to, as one translated string per section
 * (`settings.terms.<tab>.<section>`, comma-separated).
 *
 * They live in the language packs rather than in this array because a search box that only matched
 * English is a search box a translated interface cannot use: someone reading "Язык" has no reason
 * to guess that typing "language" is what finds it. Keeping them in the pack also means a
 * translator adds their own synonyms without touching code.
 */
export function sectionTermsKey(section: SettingsSectionIndex): string {
  return `settings.terms.${section.tabId}.${section.sectionId}`;
}

/** Searchable settings sections in display order. */
export const SETTINGS_SECTION_INDEX: SettingsSectionIndex[] = [
  { tabId: 'about', sectionId: 'info' },
  { tabId: 'about', sectionId: 'developer' },
  { tabId: 'appearance', sectionId: 'language' },
  { tabId: 'appearance', sectionId: 'theme' },
  { tabId: 'appearance', sectionId: 'scale' },
  // The terminal font lives under Appearance, not under a tab of its own: it was the only section
  // Terminal ever had, and a one-section tab reads as a missing feature rather than a category.
  // 'Terminal' stays in its terms so the old search still lands here.
  { tabId: 'appearance', sectionId: 'font' },
  { tabId: 'audit', sectionId: 'general' },
  { tabId: 'audit', sectionId: 'retention' },
  { tabId: 'audit', sectionId: 'privacy' },
  { tabId: 'audit', sectionId: 'secrets' },
  { tabId: 'files', sectionId: 'editor' },
  { tabId: 'files', sectionId: 'conflicts' },
  { tabId: 'hotkeys', sectionId: 'session' },
  { tabId: 'network', sectionId: 'ping' },
  { tabId: 'network', sectionId: 'transfer' },
  { tabId: 'security', sectionId: 'masterPassword' },
  { tabId: 'security', sectionId: 'lockout' },
  // 'Plugins' is in this section's terms because this is where someone searching for plugin
  // settings now lands: the Plugins screen no longer has a Security page for them to find it on.
  { tabId: 'security', sectionId: 'plugins' },
];

/** The message key holding a tab's caption. */
export function tabLabelKey(tabId: SettingsTabId): string {
  return `settings.tab.${tabId}`;
}

export function normalizeSearchQuery(query: string): string {
  return query.trim().toLowerCase();
}

export function sectionMatchesQuery(
  section: SettingsSectionIndex,
  query: string,
  translate: TermLookup,
): boolean {
  const normalized = normalizeSearchQuery(query);
  if (!normalized) return true;
  return translate(sectionTermsKey(section))
    .split(',')
    .some((term) => term.trim().toLowerCase().includes(normalized));
}

export function tabHasSearchMatches(tabId: SettingsTabId, state: SettingsSearchViewState): boolean {
  if (!normalizeSearchQuery(state.searchQuery)) return true;
  return SETTINGS_SECTION_INDEX.some(
    (section) =>
      section.tabId === tabId && sectionMatchesQuery(section, state.searchQuery, state.translate),
  );
}

/** Resolves a message key to its text. The dialog passes `$t`; tests pass whatever they need. */
export type TermLookup = (key: string) => string;

export interface SettingsSearchViewState {
  isSearching: boolean;
  activeTab: SettingsTabId;
  searchQuery: string;
  searchPinnedTab: SettingsTabId | null;
  // Carried in the view state rather than taken as a parameter so every caller re-evaluates when
  // the language changes: a `$t` read inside a reactive expression is what re-runs the filter.
  translate: TermLookup;
}

export function shouldShowSettingsSection(
  tabId: SettingsTabId,
  sectionId: string,
  state: SettingsSearchViewState,
): boolean {
  const section = SETTINGS_SECTION_INDEX.find(
    (entry) => entry.tabId === tabId && entry.sectionId === sectionId,
  );
  if (!section) return false;

  if (!state.isSearching) {
    return state.activeTab === tabId;
  }
  if (!sectionMatchesQuery(section, state.searchQuery, state.translate)) {
    return false;
  }
  if (state.searchPinnedTab && state.searchPinnedTab !== tabId) {
    return false;
  }
  return true;
}

export function shouldShowSectionTabLabel(
  tabId: SettingsTabId,
  sectionId: string,
  state: SettingsSearchViewState,
): boolean {
  if (!state.isSearching || state.searchPinnedTab) return false;
  if (!shouldShowSettingsSection(tabId, sectionId, state)) return false;

  const firstVisible = SETTINGS_SECTION_INDEX.find(
    (entry) =>
      entry.tabId === tabId && shouldShowSettingsSection(entry.tabId, entry.sectionId, state),
  );
  return firstVisible?.sectionId === sectionId;
}
