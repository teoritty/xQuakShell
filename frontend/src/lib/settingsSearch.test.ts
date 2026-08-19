// The settings search index is the only record of which tab a section lives on, so folding the
// Terminal tab into Appearance is a change to this table and nothing else. These assertions pin
// the two properties that fold had to preserve: the section still exists, and the word a user
// would actually type to find it still reaches it.
import {
  SETTINGS_SECTION_INDEX,
  SETTINGS_TAB_LABELS,
  shouldShowSettingsSection,
  tabHasSearchMatches,
  type SettingsTabId,
} from './settingsSearch';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error('FAIL: ' + msg);
}

// --- the fold: the font section moved tabs, it did not disappear ---

const fontSections = SETTINGS_SECTION_INDEX.filter((s) => s.sectionId === 'font');
assert(fontSections.length === 1, `exactly one font section, found ${fontSections.length}`);
assert(
  fontSections[0].tabId === 'appearance',
  `the terminal font section belongs to appearance, not '${fontSections[0].tabId}'`,
);

const tabIds = new Set<string>(SETTINGS_SECTION_INDEX.map((s) => s.tabId));
assert(!tabIds.has('terminal'), 'no section may still claim the removed terminal tab');
assert(
  !Object.prototype.hasOwnProperty.call(SETTINGS_TAB_LABELS, 'terminal'),
  'the terminal tab label must be gone, or the tab renders with no sections behind it',
);

// --- searching: the old word still finds the moved section ---

assert(
  tabHasSearchMatches('appearance', 'terminal font'),
  'searching "terminal font" must still surface the Appearance tab',
);
assert(
  tabHasSearchMatches('appearance', 'font size'),
  'searching "font size" must still surface the Appearance tab',
);

const searching = {
  isSearching: true,
  activeTab: 'about' as SettingsTabId,
  searchQuery: 'terminal font',
  searchPinnedTab: null,
};
assert(
  shouldShowSettingsSection('appearance', 'font', searching),
  'a "terminal font" search must render the font section even from another active tab',
);
assert(
  !shouldShowSettingsSection('appearance', 'theme', searching),
  'a "terminal font" search must not drag the unrelated theme section along with it',
);

// --- not searching: the section follows its new tab, not its old one ---

assert(
  shouldShowSettingsSection('appearance', 'font', {
    isSearching: false,
    activeTab: 'appearance',
    searchQuery: '',
    searchPinnedTab: null,
  }),
  'the font section renders when Appearance is the active tab',
);
assert(
  !shouldShowSettingsSection('appearance', 'font', {
    isSearching: false,
    activeTab: 'security',
    searchQuery: '',
    searchPinnedTab: null,
  }),
  'the font section stays hidden while another tab is active',
);

// eslint-disable-next-line no-console
console.log('settingsSearch: OK');
