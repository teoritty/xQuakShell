export type SettingsTabId =
  | 'about'
  | 'appearance'
  | 'audit'
  | 'files'
  | 'hotkeys'
  | 'network'
  | 'security'
  | 'terminal';

export interface SettingsSectionIndex {
  tabId: SettingsTabId;
  sectionId: string;
  terms: string[];
}

/** Searchable settings sections in display order. */
export const SETTINGS_SECTION_INDEX: SettingsSectionIndex[] = [
  {
    tabId: 'about',
    sectionId: 'info',
    terms: ['About', 'SSH Client', 'Version', 'Check for Updates', 'Report an Issue'],
  },
  {
    tabId: 'appearance',
    sectionId: 'theme',
    terms: ['Appearance', 'Theme', 'Dark', 'Light'],
  },
  {
    tabId: 'audit',
    sectionId: 'general',
    terms: ['Audit Log', 'General', 'Enable audit log', 'command audit logging'],
  },
  {
    tabId: 'audit',
    sectionId: 'retention',
    terms: ['Retention', 'By time', 'By count', 'Keep entries', 'Maximum entries', 'days'],
  },
  {
    tabId: 'audit',
    sectionId: 'privacy',
    terms: ['Privacy', 'Log & show username', 'Log & show connection', 'name and host', 'metadata'],
  },
  {
    tabId: 'audit',
    sectionId: 'secrets',
    terms: ['Sensitive data', 'session only', 'Log secrets this session', 'plaintext'],
  },
  {
    tabId: 'files',
    sectionId: 'editor',
    terms: ['Files', 'External Editor', 'Edit on the fly', 'Editor path'],
  },
  {
    tabId: 'hotkeys',
    sectionId: 'session',
    terms: [
      'Hotkeys',
      'Session Hotkeys',
      'Create session',
      'Next session tab',
      'Previous session tab',
      'Close active session',
      'Reset to defaults',
    ],
  },
  {
    tabId: 'network',
    sectionId: 'ping',
    terms: ['Network', 'Connection Ping', 'Enable automatic ping', 'Ping mode', 'Ping interval'],
  },
  {
    tabId: 'network',
    sectionId: 'transfer',
    terms: ['File Transfer', 'Speed limit', 'Connection timeout', 'Max concurrent transfers'],
  },
  {
    tabId: 'security',
    sectionId: 'lockout',
    terms: [
      'Security',
      'Session Lockout',
      'Enable lockout on idle timeout',
      'Idle timeout',
      'Lock when application is minimized',
    ],
  },
  {
    tabId: 'terminal',
    sectionId: 'font',
    terms: ['Terminal', 'Terminal Font', 'Font Family', 'Font Size', 'Font Color'],
  },
];

export const SETTINGS_TAB_LABELS: Record<SettingsTabId, string> = {
  about: 'About',
  appearance: 'Appearance',
  audit: 'Audit Log',
  files: 'Files',
  hotkeys: 'Hotkeys',
  network: 'Network',
  security: 'Security',
  terminal: 'Terminal',
};

export function normalizeSearchQuery(query: string): string {
  return query.trim().toLowerCase();
}

export function sectionMatchesQuery(section: SettingsSectionIndex, query: string): boolean {
  const normalized = normalizeSearchQuery(query);
  if (!normalized) return true;
  return section.terms.some((term) => term.toLowerCase().includes(normalized));
}

export function tabHasSearchMatches(tabId: SettingsTabId, query: string): boolean {
  const normalized = normalizeSearchQuery(query);
  if (!normalized) return true;
  return SETTINGS_SECTION_INDEX.some(
    (section) => section.tabId === tabId && sectionMatchesQuery(section, query),
  );
}

export interface SettingsSearchViewState {
  isSearching: boolean;
  activeTab: SettingsTabId;
  searchQuery: string;
  searchPinnedTab: SettingsTabId | null;
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
  if (!sectionMatchesQuery(section, state.searchQuery)) {
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
