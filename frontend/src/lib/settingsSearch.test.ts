// The settings search index is the only record of which tab a section lives on, so folding the
// Terminal tab into Appearance is a change to this table and nothing else. These assertions pin
// the two properties that fold had to preserve: the section still exists, and the word a user
// would actually type to find it still reaches it.
//
// The searchable words themselves now live in the language packs, so the tests read the shipped
// English pack rather than a copy of it: a term dropped from the pack must fail here, which a
// hand-written fixture could never notice.
import { readFileSync } from 'node:fs';
import {
  SETTINGS_SECTION_INDEX,
  sectionTermsKey,
  shouldShowSettingsSection,
  tabHasSearchMatches,
  tabLabelKey,
  type SettingsSearchViewState,
  type SettingsTabId,
} from './settingsSearch';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error('FAIL: ' + msg);
}

const pack = JSON.parse(
  readFileSync(new URL('../../../internal/infra/locale/builtin/en.json', import.meta.url), 'utf8'),
) as { messages: Record<string, string> };

const translate = (key: string) => pack.messages[key] ?? key;

function view(overrides: Partial<SettingsSearchViewState>): SettingsSearchViewState {
  return {
    isSearching: false,
    activeTab: 'about',
    searchQuery: '',
    searchPinnedTab: null,
    translate,
    ...overrides,
  };
}

// --- the fold: the font section moved tabs, it did not disappear ---

const fontSections = SETTINGS_SECTION_INDEX.filter((s) => s.sectionId === 'font');
assert(fontSections.length === 1, `exactly one font section, found ${fontSections.length}`);
assert(
  fontSections[0].tabId === 'appearance',
  `the terminal font section belongs to appearance, not '${fontSections[0].tabId}'`,
);

// Both removed tabs are asserted the same way: a caption with no sections behind it renders an
// empty tab, and a section with no caption renders a blank header above itself.
const tabIds = new Set<string>(SETTINGS_SECTION_INDEX.map((s) => s.tabId));
for (const gone of ['terminal', 'plugins']) {
  assert(!tabIds.has(gone), `no section may still claim the removed ${gone} tab`);
  assert(
    !(tabLabelKey(gone as SettingsTabId) in pack.messages),
    `the ${gone} tab caption must be gone, or the tab renders with no sections behind it`,
  );
}

for (const tabId of tabIds) {
  assert(
    tabLabelKey(tabId as SettingsTabId) in pack.messages,
    `sections claim tab '${tabId}', whose caption ${tabLabelKey(tabId as SettingsTabId)} is missing from the pack`,
  );
}

// Every section needs its searchable words, or it becomes unreachable through the search box.
for (const section of SETTINGS_SECTION_INDEX) {
  assert(
    sectionTermsKey(section) in pack.messages,
    `section ${section.tabId}/${section.sectionId} has no ${sectionTermsKey(section)} in the pack`,
  );
}

// --- searching: the old word still finds the moved section ---

assert(
  tabHasSearchMatches('appearance', view({ isSearching: true, searchQuery: 'terminal font' })),
  'searching "terminal font" must still surface the Appearance tab',
);
assert(
  tabHasSearchMatches('appearance', view({ isSearching: true, searchQuery: 'font size' })),
  'searching "font size" must still surface the Appearance tab',
);

const searching = view({ isSearching: true, searchQuery: 'terminal font' });
assert(
  shouldShowSettingsSection('appearance', 'font', searching),
  'a "terminal font" search must render the font section even from another active tab',
);
assert(
  !shouldShowSettingsSection('appearance', 'theme', searching),
  'a "terminal font" search must not drag the unrelated theme section along with it',
);

// The words come from the pack, so a search in the interface language has to work. Someone reading
// a Russian dialog has no reason to guess that typing "language" is what finds the language setting.
const russian = JSON.parse(
  readFileSync(new URL('../../../internal/infra/locale/builtin/ru.json', import.meta.url), 'utf8'),
) as { messages: Record<string, string> };
assert(
  shouldShowSettingsSection('appearance', 'language', {
    ...view({ isSearching: true, searchQuery: 'язык' }),
    translate: (key: string) => russian.messages[key] ?? key,
  }),
  'searching in the interface language must reach the section',
);

// --- not searching: the section follows its new tab, not its old one ---

assert(
  shouldShowSettingsSection('appearance', 'font', view({ activeTab: 'appearance' })),
  'the font section renders when Appearance is the active tab',
);
assert(
  !shouldShowSettingsSection('appearance', 'font', view({ activeTab: 'security' })),
  'the font section stays hidden while another tab is active',
);

// eslint-disable-next-line no-console
console.log('settingsSearch: OK');
