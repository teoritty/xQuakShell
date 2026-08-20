import type { GitHubPluginMetadata } from '../api/githubPlugins';

export function defaultReleaseTagForPlugin(plugin: GitHubPluginMetadata): string {
  if (plugin.installed && plugin.installedReleaseTag) {
    return plugin.installedReleaseTag;
  }
  return plugin.latestRelease || plugin.availableReleases?.[0]?.tag || '';
}

export function formatInstalledVersion(version: string): string {
  const trimmed = version.trim();
  if (!trimmed) return '';
  return trimmed.startsWith('v') ? trimmed : `v${trimmed}`;
}

/** The install state of a catalogue entry, as a message key plus the version it names. */
export function githubPluginStatusLabel(plugin: GitHubPluginMetadata): {
  kind: 'installed' | 'not-installed';
  key: string;
  vars?: Record<string, string>;
} {
  if (plugin.installed) {
    return {
      kind: 'installed',
      key: 'plugins.status.installed',
      vars: { version: formatInstalledVersion(plugin.installedVersion) },
    };
  }
  return { kind: 'not-installed', key: 'plugins.status.notInstalled' };
}

/**
 * The lines describing what is about to be installed.
 *
 * `label` resolves a message key, so this stays a pure function the tests can drive without a
 * store: the caller passes `$t` and gets back finished lines.
 */
export function githubInstallPreviewLines(
  name: string,
  releaseTag: string,
  manifestVersion: string,
  label: (key: string, vars?: Record<string, string>) => string,
): string[] {
  const lines = [
    name,
    label('plugins.preview.release', { tag: releaseTag }),
    label('plugins.preview.version', { version: manifestVersion }),
  ];
  const normalizedTag = releaseTag.replace(/^v/i, '');
  const normalizedVersion = manifestVersion.replace(/^v/i, '');
  if (normalizedTag && normalizedVersion && normalizedTag !== normalizedVersion) {
    lines.push(label('plugins.preview.tagMismatch'));
  }
  return lines;
}
