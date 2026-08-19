// View state for the Plugins screen, kept out of the components so it can be tested without
// mounting one. Everything here is a pure function of its arguments: no store reads, no RPCs.
import type { PluginInfo } from '../../api/plugins';
import type { GitHubPluginMetadata } from '../../api/githubPlugins';
import type { PluginSourceDTO } from '../../api/pluginSources';

export type PluginsSectionId = 'installed' | 'browse' | 'sources' | 'security';

export const PLUGINS_SECTION_ORDER: PluginsSectionId[] = [
  'installed',
  'browse',
  'sources',
  'security',
];

export const PLUGINS_SECTION_LABELS: Record<PluginsSectionId, string> = {
  installed: 'Installed',
  browse: 'Browse',
  sources: 'Sources',
  security: 'Security',
};

export interface RailItem {
  id: PluginsSectionId;
  label: string;
  /** Undefined where a count would be meaningless, so the badge is absent rather than showing 0. */
  count?: number;
}

/**
 * Builds the left rail.
 *
 * Security carries no badge on purpose: a number next to it would be read as a count of problems,
 * and it is a count of trusted keys.
 */
export function buildRail(installedCount: number, sourceCount: number): RailItem[] {
  return PLUGINS_SECTION_ORDER.map((id) => ({
    id,
    label: PLUGINS_SECTION_LABELS[id],
    count: id === 'installed' ? installedCount : id === 'sources' ? sourceCount : undefined,
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

/** True when installing from this source should be offered at all. */
export function canInstallFrom(source: PluginSourceDTO): boolean {
  return source.available;
}

export interface BrowseGroup {
  source: PluginSourceDTO;
  plugins: GitHubPluginMetadata[];
  error?: string;
  loading: boolean;
}

/**
 * Assembles the Browse list: one group per source, in source order, with the search applied.
 *
 * A group whose plugins all fail the search is dropped, but a group that is loading, errored or
 * unavailable is kept: those three states are about the source rather than its contents, and
 * hiding them would make a failing repository look like a repository with nothing in it.
 */
export function buildBrowseGroups(
  sources: PluginSourceDTO[],
  pluginsBySource: Record<string, GitHubPluginMetadata[]>,
  errorsBySource: Record<string, string>,
  loadingSources: Record<string, boolean>,
  query: string,
): BrowseGroup[] {
  const groups: BrowseGroup[] = [];
  for (const source of sources) {
    const loading = loadingSources[source.id] === true;
    const error = errorsBySource[source.id];
    const plugins = filterCatalog(pluginsBySource[source.id] ?? [], query);

    if (plugins.length === 0 && !loading && !error && source.available) continue;
    groups.push({ source, plugins, error, loading });
  }
  return groups;
}
