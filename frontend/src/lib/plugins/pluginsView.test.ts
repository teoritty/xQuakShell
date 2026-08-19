// The Plugins screen's rail, search and grouping, asserted without mounting a component.
//
// The load-bearing case is buildBrowseGroups: a source that is unavailable, erroring or still
// loading must survive a search that its (absent) contents cannot match, because those three
// states are properties of the source rather than of its plugins. Dropping them is how a broken
// repository comes to look like an empty one.
import type { PluginSourceDTO } from '../../api/pluginSources';
import type { GitHubPluginMetadata } from '../../api/githubPlugins';
import type { PluginInfo } from '../../api/plugins';
import {
  buildBrowseGroups,
  buildRail,
  canInstallFrom,
  filterInstalled,
  matchesQuery,
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

// --- the rail ---

const rail = buildRail(4, 3);
assert(rail.length === 4, `rail has ${rail.length} items, want 4`);
assert(rail[0].id === 'installed' && rail[0].count === 4, 'Installed carries its count');
assert(rail[2].id === 'sources' && rail[2].count === 3, 'Sources carries its count');
assert(
  rail[3].id === 'security' && rail[3].count === undefined,
  'Security carries no badge: a number there reads as a count of problems, not of keys',
);
assert(rail[1].count === undefined, 'Browse has nothing to count until a source has answered');

// --- search ---

assert(matchesQuery(['SFTP Sync'], ''), 'an empty query matches everything');
assert(matchesQuery(['SFTP Sync'], '  '), 'a whitespace query is an empty query');
assert(matchesQuery(['SFTP Sync'], 'sftp'), 'search is case-insensitive');
assert(!matchesQuery(['SFTP Sync'], 'rsync'), 'a non-substring does not match');
assert(matchesQuery([undefined, 'sync'], 'sync'), 'an absent field is skipped, not a crash');

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
    ...over,
  };
}

const installed: PluginInfo[] = [
  plugin(),
  plugin({ id: 'b', name: 'K8s Tunnel', description: 'clusters', source: 'https://gitlab.com/x/y' }),
];
assert(filterInstalled(installed, 'k8s').length === 1, 'installed search narrows by name');
assert(filterInstalled(installed, 'clusters')[0].id === 'b', 'installed search covers description');
assert(
  filterInstalled(installed, 'gitlab.com')[0].id === 'b',
  'installed search covers source: "which of these came from that repository" precedes removing it',
);
assert(filterInstalled(installed, '').length === 2, 'an empty search keeps every installed plugin');

// --- source status ---

assert(sourceStatus(source()).kind === 'trusted', 'an available trusted source reads as trusted');
assert(sourceStatus(source({ trusted: false })).kind === 'untrusted', 'and untrusted when it is');
const down = sourceStatus(source({ available: false, trusted: false, unavailableReason: 'nope' }));
assert(
  down.kind === 'unavailable' && down.text === 'nope',
  'unavailable outranks untrusted, and shows the backend reason verbatim',
);
assert(
  sourceStatus(source({ available: false })).text === 'Unavailable',
  'a missing reason still produces a label rather than an empty badge',
);

assert(canInstallFrom(source()), 'an available source can be installed from');
assert(!canInstallFrom(source({ available: false })), 'an unavailable source cannot');

// --- browse grouping ---

const forge = source({ id: 'repo-1' });
const market = source({
  id: 'market',
  kind: 'marketplace',
  available: false,
  removable: false,
  unavailableReason: 'not serving yet',
});

const groups = buildBrowseGroups(
  [forge, market],
  { 'repo-1': [entry()] },
  {},
  {},
  'sftp',
);
assert(groups.length === 2, `got ${groups.length} groups, want the forge match and the dead source`);
assert(groups[0].source.id === 'repo-1' && groups[0].plugins.length === 1, 'the match is present');
assert(
  groups[1].source.id === 'market' && groups[1].plugins.length === 0,
  'an unavailable source is listed with no plugins, not dropped',
);

const noMatch = buildBrowseGroups([forge, market], { 'repo-1': [entry()] }, {}, {}, 'nothing');
assert(
  noMatch.length === 1 && noMatch[0].source.id === 'market',
  'an available source whose plugins all fail the search is dropped; the unavailable one stays',
);

const erroring = buildBrowseGroups([forge], {}, { 'repo-1': 'rate limited' }, {}, 'zzz');
assert(
  erroring.length === 1 && erroring[0].error === 'rate limited',
  'an errored source survives a search it cannot match, or the failure is invisible',
);

const loading = buildBrowseGroups([forge], {}, {}, { 'repo-1': true }, 'zzz');
assert(
  loading.length === 1 && loading[0].loading,
  'a loading source survives a search, or the row flickers out mid-fetch',
);

const quiet = buildBrowseGroups([forge], {}, {}, {}, '');
assert(quiet.length === 0, 'an available, idle, empty source contributes no group');

// eslint-disable-next-line no-console
console.log('pluginsView: OK');
