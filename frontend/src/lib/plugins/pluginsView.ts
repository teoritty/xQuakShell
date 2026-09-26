// View state for the Plugins screen, kept out of the components so it can be tested without
// mounting one. Everything here is a pure function of its arguments: no store reads, no RPCs.
import type { PluginInfo } from '../../api/plugins';
import type { GitHubPluginMetadata } from '../../api/githubPlugins';
import type { PluginSourceDTO } from '../../api/pluginSources';

// The trust policy is deliberately absent: it lives in Settings → Security, with the lockout and
// the rest of the installation-wide policy, rather than in a screen about individual plugins.
export type PluginsSectionId = 'installed' | 'browse' | 'sources' | 'marketplace';

// Marketplace is left out of the rail while it has nothing to show but "coming soon". The section
// and its page stay, so bringing it back is adding the id here again.
export const PLUGINS_SECTION_ORDER: PluginsSectionId[] = [
  'installed',
  'browse',
  'sources',
];

/** The message key holding a section's caption. */
export function sectionLabelKey(id: PluginsSectionId): string {
  return `plugins.section.${id}`;
}

export interface RailItem {
  id: PluginsSectionId;
  labelKey: string;
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
 * Installed carries an alert dot instead of folding the attention count into its badge - "4" and
 * "4, one of which is broken" must not look the same. Browse and Marketplace carry no count: what
 * they hold is a remote answer that may not have arrived, and a badge would report a fetch state
 * as a quantity.
 */
export function buildRail(counts: RailCounts): RailItem[] {
  return PLUGINS_SECTION_ORDER.map((id) => ({
    id,
    labelKey: sectionLabelKey(id),
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
 * Installed plugins are searched by install source as well as by name, which is a coarser field
 * than it sounds: the backend sends the InstallSource enum, so the only thing it can answer is
 * "bundled or not". It stays in the haystack for that one query and is not shown on the card,
 * where a row reading "user" under every name says nothing.
 */
export function filterInstalled(plugins: PluginInfo[], query: string): PluginInfo[] {
  return plugins.filter((p) => matchesQuery([p.id, p.name, p.description, p.source], query));
}

/** True for a plugin that ships with the app rather than one the user installed. */
export function isBundled(plugin: PluginInfo): boolean {
  return plugin.source === 'bundled';
}

/**
 * The run state as a user reads it: a message key where there is one, otherwise the backend's own
 * word capitalised.
 *
 * `discovered` is the registry's word for "loaded, never started", and on a card it reads as a
 * finding about the plugin rather than as a state - which is why it is the one word given a key.
 * Every other state is passed through, so a state the backend adds later surfaces under its own
 * name instead of being swallowed by a default. That also means it stays English: this layer
 * cannot translate a word it has never seen.
 */
export function pluginStateLabel(state: string): { key?: string; text?: string } {
  if (!state || state === 'discovered') return { key: 'plugins.state.notRunning' };
  return { text: state.charAt(0).toUpperCase() + state.slice(1) };
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
  labelKey: string;
  /** The key of one line under the header saying what membership of this group means. */
  hintKey: string;
  plugins: PluginInfo[];
}

const INSTALLED_GROUPS: { id: PluginRunState; labelKey: string; hintKey: string }[] = [
  {
    id: 'attention',
    labelKey: 'plugins.group.attention',
    hintKey: 'plugins.group.attention.hint',
  },
  { id: 'active', labelKey: 'plugins.group.active', hintKey: 'plugins.group.active.hint' },
  { id: 'disabled', labelKey: 'plugins.group.disabled', hintKey: 'plugins.group.disabled.hint' },
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
  labelKey: string;
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
      return { labelKey: 'security.plugin.sandbox.enforced', tone: 'good' };
    case 'enforced-partial':
      return { labelKey: 'security.plugin.sandbox.partial', tone: 'warn' };
    case 'unavailable':
      return { labelKey: 'security.plugin.sandbox.unavailable', tone: 'bad' };
    case 'disabled':
      return { labelKey: 'security.plugin.sandbox.disabled', tone: 'bad' };
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

/**
 * The Sources list under search, matched on the URL as well as the display name.
 *
 * The URL is what the user pasted to register the repository, so it is what they type to find it
 * again - and the display name is derived from it, which makes an owner/name query miss whenever
 * the two have drifted.
 */
export function filterSources(sources: PluginSourceDTO[], query: string): PluginSourceDTO[] {
  return forgeSources(sources).filter((s) => matchesQuery([s.id, s.displayName], query));
}

export interface SourceStatus {
  kind: 'unavailable' | 'untrusted' | 'trusted';
  /** A message key, except where the backend supplied its own reason in `text`. */
  textKey?: string;
  text?: string;
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
    return source.unavailableReason
      ? { kind: 'unavailable', text: source.unavailableReason }
      : { kind: 'unavailable', textKey: 'plugins.source.unavailable' };
  }
  return source.trusted
    ? { kind: 'trusted', textKey: 'security.plugin.source.trusted' }
    : { kind: 'untrusted', textKey: 'security.plugin.source.untrusted' };
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
