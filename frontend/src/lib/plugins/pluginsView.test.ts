// The Plugins screen's rail, grouping, search and source partitioning, asserted without mounting
// a component.
//
// Two cases carry most of the weight. groupInstalled decides what a user sees first, and a
// disabled plugin must never be promoted to "needs attention" - it is not running, so whatever is
// wrong with it is not happening now, and promoting it buries the one that is failing.
// forgeSources decides that the marketplace never appears in a list whose every control would be
// inert on it.
import type { PluginSourceDTO } from '../../api/pluginSources';
import type { GitHubPluginMetadata } from '../../api/githubPlugins';
import type { PluginInfo } from '../../api/plugins';
import {
  buildBrowseGroups,
  buildRail,
  countNeedsAttention,
  filterInstalled,
  filterSources,
  forgeSources,
  groupInstalled,
  hasUpdate,
  isBundled,
  matchesQuery,
  pluginRunState,
  pluginStateLabel,
  sandboxSummary,
  sourceStatus,
} from './pluginsView';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error('FAIL: ' + msg);
}

function source(over: Partial<PluginSourceDTO> = {}): PluginSourceDTO {
  return {
    id: 'https://github.com/o/r',
    kind: 'forge',
    displayName: 'o/r',
    trusted: true,
    removable: true,
    available: true,
    ...over,
  };
}

const market = source({
  id: 'https://api.xquakshell.ru',
  kind: 'marketplace',
  displayName: 'xQuakShell Marketplace',
  removable: false,
  available: false,
  unavailableReason: 'not serving yet',
});

function entry(over: Partial<GitHubPluginMetadata> = {}): GitHubPluginMetadata {
  return {
    repositoryUrl: 'https://github.com/o/r',
    id: 'sftp-sync',
    name: 'SFTP Sync',
    version: '1.2.0',
    description: 'Two-way folder sync',
    author: 'teoritty',
    license: 'MIT',
    platforms: [],
    availableReleases: [],
    latestRelease: 'v1.2.0',
    prerelease: false,
    publishedAt: '',
    readme: '',
    platformSupported: true,
    installed: false,
    installedVersion: '',
    installedReleaseTag: '',
    ...over,
  };
}

function plugin(over: Partial<PluginInfo> = {}): PluginInfo {
  return {
    id: 'a',
    name: 'SFTP Sync',
    version: '1.2.0',
    description: 'folders',
    source: 'https://github.com/teoritty/sftp-sync',
    state: 'running',
    requiresSecretAccess: false,
    signed: true,
    enabled: true,
    sandboxMode: 'enforced',
    ...over,
  };
}

// --- run state ---

assert(pluginRunState(plugin()) === 'active', 'enabled, running and sandboxed is active');
assert(pluginRunState(plugin({ enabled: false })) === 'disabled', 'disabled outranks everything');
assert(
  pluginRunState(plugin({ enabled: false, state: 'crashed', sandboxMode: 'unavailable' })) ===
    'disabled',
  'a disabled plugin is never promoted to attention: it is not running, so nothing is happening',
);
assert(
  pluginRunState(plugin({ state: 'stopped' })) === 'attention',
  'enabled but not running needs attention',
);
assert(
  pluginRunState(plugin({ sandboxMode: 'unavailable' })) === 'attention',
  'running with no sandbox needs attention',
);
assert(
  pluginRunState(plugin({ sandboxMode: 'enforced-partial' })) === 'attention',
  'partial confinement needs attention and must never round up to active',
);

// --- grouping ---

const installed = [
  plugin({ id: 'a', name: 'Zeta' }),
  plugin({ id: 'b', name: 'Alpha' }),
  plugin({ id: 'c', name: 'Broken', state: 'stopped' }),
  plugin({ id: 'd', name: 'Off', enabled: false }),
];

const groups = groupInstalled(installed, '');
assert(groups.length === 3, `got ${groups.length} groups, want attention/active/disabled`);
assert(groups[0].id === 'attention', 'attention comes first: the top of the list is what gets read');
assert(groups[1].id === 'active' && groups[2].id === 'disabled', 'then active, then disabled');
assert(
  groups[1].plugins.map((p) => p.name).join(',') === 'Alpha,Zeta',
  `active group order was ${groups[1].plugins.map((p) => p.name)}; within a group, sort by name`,
);
assert(countNeedsAttention(installed) === 1, 'one plugin needs attention');

const healthy = groupInstalled([plugin()], '');
assert(
  healthy.length === 1 && healthy[0].id === 'active',
  'empty groups are dropped: a permanent "Needs attention (0)" teaches people to ignore it',
);

const searched = groupInstalled(installed, 'alpha');
assert(
  searched.length === 1 && searched[0].plugins.length === 1,
  'search narrows the groups and drops the ones left empty',
);

// --- sandbox chip ---

assert(sandboxSummary('enforced')?.tone === 'good', 'full confinement reads good');
assert(sandboxSummary('enforced-partial')?.tone === 'warn', 'partial confinement reads as a warning');
assert(
  sandboxSummary('enforced-partial')?.labelKey !== sandboxSummary('enforced')?.labelKey,
  'partial confinement must not claim the containment full confinement does',
);
assert(sandboxSummary('unavailable')?.tone === 'bad', 'no sandbox reads bad');
assert(sandboxSummary(undefined) === null, 'a plugin that is not running has no sandbox chip');

// --- rail ---

const rail = buildRail({ installed: 4, sources: 3, needsAttention: 1 });
assert(rail.length === 4, `rail has ${rail.length} items, want 4`);
assert(rail[0].id === 'installed' && rail[0].count === 4, 'Installed carries its count');
assert(rail[0].alert === true, 'Installed shows the alert dot when something needs attention');
assert(rail[2].id === 'sources' && rail[2].count === 3, 'Sources carries its count');
assert(
  rail[3].id === 'marketplace' && rail[3].count === undefined,
  'the marketplace is its own destination and carries no count',
);
assert(
  rail.map((item) => item.id).join(',') === 'installed,browse,sources,marketplace',
  'the rail is those four sections; the trust policy lives in Settings, not here',
);
assert(
  buildRail({ installed: 4, sources: 3, needsAttention: 0 })[0].alert === false,
  'no alert dot when nothing needs attention',
);

// --- search ---

assert(matchesQuery(['SFTP Sync'], ''), 'an empty query matches everything');
assert(matchesQuery(['SFTP Sync'], '  '), 'a whitespace query is an empty query');
assert(matchesQuery(['SFTP Sync'], 'sftp'), 'search is case-insensitive');
assert(!matchesQuery(['SFTP Sync'], 'rsync'), 'a non-substring does not match');
assert(matchesQuery([undefined, 'sync'], 'sync'), 'an absent field is skipped, not a crash');
assert(
  filterInstalled([plugin({ source: 'https://gitlab.com/x/y' })], 'gitlab.com').length === 1,
  'installed search covers source: "which came from that repository" precedes removing it',
);

// --- source partitioning ---

const all = [source({ id: 'repo-1' }), market, source({ id: 'repo-2' })];
assert(forgeSources(all).length === 2, 'forgeSources keeps only the registered repositories');
assert(
  forgeSources(all).every((s) => s.kind === 'forge'),
  'the marketplace never reaches a list whose trust, refresh and remove controls are inert on it',
);

assert(
  filterSources(all, 'repo-2').map((s) => s.id).join(',') === 'repo-2',
  'a source search matches the URL, which is what the user pasted to register it',
);
assert(
  filterSources(all, '').length === 2,
  'an empty source search is the whole list, still without the marketplace',
);

// --- install source and run state, as a user reads them ---

assert(isBundled(plugin({ source: 'bundled' })), 'a plugin shipped with the app is bundled');
assert(!isBundled(plugin({ source: 'user' })), 'and one the user installed is not');
assert(
  pluginStateLabel('discovered').key === 'plugins.state.notRunning',
  'the registry word for "loaded, never started" is not shown to the user verbatim',
);
assert(
  pluginStateLabel('').key === 'plugins.state.notRunning',
  'and neither is an absent state shown as blank',
);
assert(
  pluginStateLabel('crashed').text === 'Crashed' && pluginStateLabel('crashed').key === undefined,
  'every other state keeps its own word, so a state added later is not swallowed by a default',
);

// --- source status ---

assert(sourceStatus(source()).kind === 'trusted', 'an available trusted source reads as trusted');
assert(sourceStatus(source({ trusted: false })).kind === 'untrusted', 'and untrusted when it is');
const down = sourceStatus(source({ available: false, trusted: false, unavailableReason: 'nope' }));
assert(
  down.kind === 'unavailable' && down.text === 'nope' && down.textKey === undefined,
  'unavailable outranks untrusted, and shows the backend reason verbatim rather than a key',
);
assert(
  sourceStatus(source({ available: false })).textKey === 'plugins.source.unavailable',
  'a missing reason still produces a label rather than an empty badge',
);

// --- browse grouping ---

const forge = source({ id: 'repo-1' });

const browse = buildBrowseGroups([forge, market], { 'repo-1': [entry()] }, {}, {}, 'sftp');
assert(
  browse.length === 1 && browse[0].source.id === 'repo-1',
  'Browse lists repositories only; the marketplace has its own page',
);

const noMatch = buildBrowseGroups([forge], { 'repo-1': [entry()] }, {}, {}, 'nothing');
assert(noMatch.length === 0, 'a repository whose plugins all fail the search is dropped');

const erroring = buildBrowseGroups([forge], {}, { 'repo-1': 'rate limited' }, {}, 'zzz');
assert(
  erroring.length === 1 && erroring[0].error === 'rate limited',
  'an errored repository survives a search it cannot match, or the failure is invisible',
);

const loading = buildBrowseGroups([forge], {}, {}, { 'repo-1': true }, 'zzz');
assert(
  loading.length === 1 && loading[0].loading,
  'a loading repository survives a search, or the row flickers out mid-fetch',
);

// --- updates ---

assert(
  hasUpdate(entry({ installed: true, installedReleaseTag: 'v1.1.0', latestRelease: 'v1.2.0' })),
  'an installed plugin behind the newest release has an update',
);
assert(
  !hasUpdate(entry({ installed: true, installedReleaseTag: 'v1.2.0', latestRelease: 'v1.2.0' })),
  'a plugin on the newest release has none',
);
assert(!hasUpdate(entry({ installed: false })), 'a plugin that is not installed cannot have one');
assert(
  !hasUpdate(entry({ installed: true, installedReleaseTag: '', latestRelease: 'v1.2.0' })),
  'an unknown installed tag is not an update: it is missing information, and offering "Update" ' +
    'from it would reinstall on every open',
);

// eslint-disable-next-line no-console
console.log('pluginsView: OK');
