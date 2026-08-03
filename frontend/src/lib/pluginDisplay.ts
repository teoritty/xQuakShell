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

export function githubPluginStatusLabel(plugin: GitHubPluginMetadata): {
  kind: 'installed' | 'not-installed';
  text: string;
} {
  if (plugin.installed) {
    return {
      kind: 'installed',
      text: `Installed ${formatInstalledVersion(plugin.installedVersion)}`,
    };
  }
  return { kind: 'not-installed', text: 'Not installed' };
}

export function githubInstallPreviewLines(
  name: string,
  releaseTag: string,
  manifestVersion: string,
): string[] {
  const lines = [name, `Release: ${releaseTag}`, `Version: ${manifestVersion}`];
  const normalizedTag = releaseTag.replace(/^v/i, '');
  const normalizedVersion = manifestVersion.replace(/^v/i, '');
  if (normalizedTag && normalizedVersion && normalizedTag !== normalizedVersion) {
    lines.push('Tag and manifest version differ');
  }
  return lines;
}
