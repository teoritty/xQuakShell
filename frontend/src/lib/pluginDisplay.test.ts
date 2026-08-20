import type { GitHubPluginMetadata } from '../api/githubPlugins';
import {
  defaultReleaseTagForPlugin,
  formatInstalledVersion,
  githubInstallPreviewLines,
  githubPluginStatusLabel,
} from './pluginDisplay';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

function plugin(overrides: Partial<GitHubPluginMetadata> = {}): GitHubPluginMetadata {
  return {
    repositoryUrl: 'https://github.com/user/repo',
    id: 'com.example.demo',
    name: 'Demo',
    version: '1.0.0',
    description: '',
    author: '',
    license: '',
    platforms: [],
    availableReleases: [{ tag: 'v1.0.1', name: '', publishedAt: '', prerelease: false, platformSupported: true, platforms: [] }],
    latestRelease: 'v1.0.1',
    prerelease: false,
    publishedAt: '',
    readme: '',
    platformSupported: true,
    installed: false,
    installedVersion: '',
    installedReleaseTag: '',
    ...overrides,
  };
}

assert(defaultReleaseTagForPlugin(plugin()) === 'v1.0.1', 'defaults release tag to latest when not installed');
assert(
  defaultReleaseTagForPlugin(plugin({ installed: true, installedReleaseTag: 'v1.0.0', installedVersion: '1.0.0' })) === 'v1.0.0',
  'defaults release tag to installed tag when installed',
);
assert(formatInstalledVersion('1.0.0') === 'v1.0.0', 'formats installed version with v prefix');
assert(formatInstalledVersion('v1.0.1') === 'v1.0.1', 'keeps existing v prefix');
assert(
  JSON.stringify(githubPluginStatusLabel(plugin())) ===
    JSON.stringify({ kind: 'not-installed', key: 'plugins.status.notInstalled' }),
  'shows not installed status',
);
assert(
  JSON.stringify(githubPluginStatusLabel(plugin({ installed: true, installedVersion: '1.0.0' }))) ===
    JSON.stringify({ kind: 'installed', key: 'plugins.status.installed', vars: { version: 'v1.0.0' } }),
  'shows installed status from backend version',
);

// The lines are assembled through a lookup, so the test drives it with an identity one: what is
// being asserted is which lines appear, not how any language words them.
const echo = (key: string) => key;
assert(
  githubInstallPreviewLines('Telnet', 'v1.0.1', '1.0.0', echo).includes('plugins.preview.tagMismatch'),
  'flags mismatched release tag and manifest version',
);
assert(
  !githubInstallPreviewLines('Telnet', 'v1.0.0', '1.0.0', echo).includes('plugins.preview.tagMismatch'),
  'and stays quiet when the tag and the manifest version agree',
);

console.log('pluginDisplay.test.ts: all passed');
