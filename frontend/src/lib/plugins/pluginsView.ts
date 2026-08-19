// View state for the Plugins screen, kept out of the components so it can be tested without
// mounting one. Everything here is a pure function of its arguments: no store reads, no RPCs.
import type { PluginInfo } from '../../api/plugins';
import type { GitHubPluginMetadata } from '../../api/githubPlugins';
import type { PluginSourceDTO } from '../../api/pluginSources';

export type PluginsSectionId =
  | 'installed'
  | 'browse'
  | 'sources'
  | 'marketplace'
  | 'security';

export const PLUGINS_SECTION_ORDER: PluginsSectionId[] = [
  'installed',
  'browse',
  'sources',
  'marketplace',
  'security',
];

export const PLUGINS_SECTION_LABELS: Record<PluginsSectionId, string> = {
  installed: 'Installed',
  browse: 'Browse',
  sources: 'Sources',
  marketplace: 'Marketplace',
  security: 'Security',
};

export interface RailItem {
  id: PluginsSectionId;
  label: string;
  /** Undefined where a count would be meaningless, so the badge is absent rather than showing 0. */
  count?: number;
  /** Draws the attention dot. Set only where something is actionable right now. */
  alert?: boolean;
}

export interface RailCounts {
  installed: number;
  sources: number;
  needsAttention: number;
}

/**
 * Builds the left rail.
 *
 * Security carries no count on purpose: a number next to it would be read as a count of problems,
 * and it is a count of trusted keys. Installed carries an alert dot instead of folding the
 * attention count into its badge - "4" and "4, one of which is broken" must not look the same.
 */
export function buildRail(counts: RailCounts): RailItem[] {
  return PLUGINS_SECTION_ORDER.map((id) => ({
    id,
    label: PLUGINS_SECTION_LABELS[id],
    count: id === 'installed' ? counts.installed : id === 'sources' ? counts.sources : undefined,
    alert: id === 'installed' && counts.needsAttention > 0,
  }));
}

export function normalizeQuery(query: string): string {
  return query.trim().toLowerCase();
}

/** True when any of the haystack fields contains the query. An empty query matches everything. */
export function matchesQuery(haystack: (string | undefined)[], query: string): boolean {
  const needle = normalizeQuery(query);
  if (!needle) return true;
  return haystack.some((field) => (field ?? '').toLowerCase().includes(needle));
}

/**
 * Installed plugins are searched by where they came from as well as by name: `source` is the only
 * field that answers "which of these did I get from that repository", which is the question a user
 * asks right before removing a source.
 */
export function filterInstalled(plugins: PluginInfo[], query: string): PluginInfo[] {
  return plugins.filter((p) => matchesQuery([p.id, p.name, p.description, p.source], query));
}

export function filterCatalog(
  entries: GitHubPluginMetadata[],
  query: string,
): GitHubPluginMetadata[] {
  return entries.filter((e) => matchesQuery([e.id, e.name, e.description, e.author], query));
}

function byName<T extends { name?: string; id: string }>(a: T, b: T): number {
  return (a.name || a.id).localeCompare(b.name || b.id, undefined, { sensitivity: 'base' });
}

// --- installed: state, not a pile ---

export type PluginRunState = 'attention' | 'active' | 'disabled';

/**
 * Which of the three installed groups a plugin belongs in.
 *
 * A disabled plugin is never "attention" however broken it looks: it is not running, so whatever
 * is wrong with it is not currently affecting anything, and putting it at the top would bury the
 * plugin that is failing right now.
 */
export function pluginRunState(plugin: PluginInfo): PluginRunState {
  if (!plugin.enabled) return 'disabled';
  if (plugin.state !== 'running') return 'attention';
  if (plugin.sandboxMode === 'unavailable' || plugin.sandboxMode === 'enforced-partial') {
    return 'attention';
  }
  return 'active';
}

export interface InstalledGroup {
  id: PluginRunState;
  label: string;
  /** One line under the header saying what membership of this group means. */
  hint: string;
  plugins: PluginInfo[];
}

const INSTALLED_GROUPS: { id: PluginRunState; label: string; hint: string }[] = [
  {
    id: 'attention',
    label: 'Needs attention',
    hint: 'Enabled, but not running as intended.',
  },
  { id: 'active', label: 'Active', hint: 'Running and confined.' },
  { id: 'disabled', label: 'Disabled', hint: 'Installed, not started.' },
];

/**
 * Groups the installed list by run state, attention first.
 *
 * Order is by urgency rather than alphabetical because the top of this list is the only part a
 * user reliably reads. Empty groups are dropped: a permanently visible "Needs attention (0)"
 * teaches people to stop looking at it.
 */
export function groupInstalled(plugins: PluginInfo[], query: string): InstalledGroup[] {
  const matched = filterInstalled(plugins, query);
  return INSTALLED_GROUPS.map((group) => ({
    ...group,
    plugins: matched.filter((p) => pluginRunState(p) === group.id).sort(byName),
  })).filter((group) => group.plugins.length > 0);
}

export function countNeedsAttention(plugins: PluginInfo[]): number {
  return plugins.filter((p) => pluginRunState(p) === 'attention').length;
}

export interface SandboxSummary {
  label: string;
  tone: 'good' | 'warn' | 'bad';
}

/**
 * How contained a plugin's processes are, in one chip.
 *
 * `enforced-partial` is a real boundary with a dimension missing - a Linux kernel below 6.7
 * confines the filesystem and not the network - so it reads as a warning and never rounds up to
 * "Sandboxed". Claiming containment the user does not have is the one failure this chip must not
 * have.
 */
export function sandboxSummary(mode: string | undefined): SandboxSummary | null {
  switch (mode) {
    case 'enforced':
      return { label: 'Sandboxed', tone: 'good' };
    case 'enforced-partial':
      return { label: 'Partly sandboxed', tone: 'warn' };
    case 'unavailable':
      return { label: 'No sandbox', tone: 'bad' };
    case 'disabled':
      return { label: 'Sandbox off', tone: 'bad' };
    default:
      return null;
  }
}

// --- sources ---

/**
 * The registered repositories, without the built-in marketplace row.
 *
 * The marketplace has its own section, and mixing it into a list whose every other entry can be
 * trusted, refreshed or removed offers three controls that do nothing to it.
 */
export function forgeSources(sources: PluginSourceDTO[]): PluginSourceDTO[] {
  return sources.filter((s) => s.kind === 'forge');
}

export function marketplaceSource(sources: PluginSourceDTO[]): PluginSourceDTO | null {
  return sources.find((s) => s.kind === 'marketplace') ?? null;
}

export interface SourceStatus {
  kind: 'unavailable' | 'untrusted' | 'trusted';
  text: string;
}

/**
 * Describes a source in one badge.
 *
 * Unavailable outranks untrusted deliberately. A source that cannot answer at all is the fact that
 * explains why its rows are empty, and showing "untrusted" there would send the user to fix a
 * trust setting that changes nothing.
 */
export function sourceStatus(source: PluginSourceDTO): SourceStatus {
  if (!source.available) {
    return { kind: 'unavailable', text: source.unavailableReason || 'Unavailable' };
  }
  return source.trusted
    ? { kind: 'trusted', text: 'Trusted' }
    : { kind: 'untrusted', text: 'Not trusted' };
}

export interface BrowseGroup {
  source: PluginSourceDTO;
  plugins: GitHubPluginMetadata[];
  error?: string;
  loading: boolean;
}

/**
 * Assembles the Browse list: one group per registered repository, in source order, search applied.
 *
 * A group whose plugins all fail the search is dropped, but a group that is loading or errored is
 * kept: both states are about the source rather than its contents, and hiding them would make a
 * failing repository look like a repository with nothing in it.
 */
export function buildBrowseGroups(
  sources: PluginSourceDTO[],
  pluginsBySource: Record<string, GitHubPluginMetadata[]>,
  errorsBySource: Record<string, string>,
  loadingSources: Record<string, boolean>,
  query: string,
): BrowseGroup[] {
  const groups: BrowseGroup[] = [];
  for (const source of forgeSources(sources)) {
    const loading = loadingSources[source.id] === true;
    const error = errorsBySource[source.id];
    const plugins = filterCatalog(pluginsBySource[source.id] ?? [], query).sort(byName);

    if (plugins.length === 0 && !loading && !error) continue;
    groups.push({ source, plugins, error, loading });
  }
  return groups;
}

/**
 * True when an installed plugin is behind the newest release its source offers.
 *
 * Compared on the release tag rather than the manifest version: the tag is what an install is
 * pinned to, and a plugin whose manifest version was never bumped between two tags is still a
 * different build.
 */
export function hasUpdate(plugin: GitHubPluginMetadata): boolean {
  if (!plugin.installed || !plugin.latestRelease) return false;
  return plugin.installedReleaseTag !== '' && plugin.installedReleaseTag !== plugin.latestRelease;
}
