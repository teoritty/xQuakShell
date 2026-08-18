import { parseGitHubRepoRef, resolveGitHubReadmeUrl, type GitHubRepoRef } from './githubReadme';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

function assertRef(url: string, want: GitHubRepoRef | null, msg: string) {
  const got = parseGitHubRepoRef(url);
  assert(JSON.stringify(got) === JSON.stringify(want), `${msg}: got ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
}

assertRef(
  'https://github.com/teoritty/xQuakShell',
  { forge: 'github', owner: 'teoritty', repo: 'xQuakShell' },
  'a github.com URL is a github ref',
);
assertRef(
  'https://gitlab.com/teoritty/xQuakShell',
  { forge: 'gitlab', owner: 'teoritty', repo: 'xQuakShell' },
  'a gitlab.com URL is a gitlab ref',
);

// A GitLab namespace is any number of segments. Keeping only the first two addresses a project that
// does not exist, and every relative image in the README then 404s.
assertRef(
  'https://gitlab.com/group/subgroup/plugin',
  { forge: 'gitlab', owner: 'group/subgroup', repo: 'plugin' },
  'a GitLab subgroup namespace is kept whole',
);

// "/-/" is the only thing separating a subgroup from an in-project route.
assertRef(
  'https://gitlab.com/group/plugin/-/tree/main',
  { forge: 'gitlab', owner: 'group', repo: 'plugin' },
  'parsing stops at the GitLab route separator',
);

// GitHub's namespace is exactly one segment, so a deeper path is a page inside the repository.
assertRef(
  'https://github.com/teoritty/xQuakShell/tree/main/docs',
  { forge: 'github', owner: 'teoritty', repo: 'xQuakShell' },
  'a GitHub page path does not become a namespace',
);

assertRef('https://bitbucket.org/team/plugin', null, 'an unsupported host has no ref');
assertRef('not a url', null, 'an unparseable URL has no ref');

// The two forges serve raw bytes from completely different URLs: a separate host on GitHub, a route
// on the repository itself on GitLab. Using GitHub's shape for a GitLab repo yields a URL on a host
// that has never heard of the project.
assert(
  resolveGitHubReadmeUrl('./docs/demo.png', 'teoritty', 'plugin', 'v1.0.0', 'github') ===
    'https://raw.githubusercontent.com/teoritty/plugin/v1.0.0/docs/demo.png',
  'a relative path resolves against raw.githubusercontent.com on GitHub',
);
assert(
  resolveGitHubReadmeUrl('./docs/demo.png', 'teoritty', 'plugin', 'v1.0.0', 'gitlab') ===
    'https://gitlab.com/teoritty/plugin/-/raw/v1.0.0/docs/demo.png',
  'a relative path resolves against the /-/raw/ route on GitLab',
);

assert(
  resolveGitHubReadmeUrl('https://github.com/teoritty/plugin/blob/main/demo.png', 'teoritty', 'plugin', 'v1.0.0', 'github') ===
    'https://raw.githubusercontent.com/teoritty/plugin/main/demo.png',
  'a GitHub blob link is rewritten to raw bytes',
);
assert(
  resolveGitHubReadmeUrl('https://gitlab.com/group/sub/plugin/-/blob/main/demo.png', 'group/sub', 'plugin', 'v1.0.0', 'gitlab') ===
    'https://gitlab.com/group/sub/plugin/-/raw/main/demo.png',
  'a GitLab blob link is rewritten to raw bytes',
);

assert(
  resolveGitHubReadmeUrl('demo.png', 'o', 'r', '', 'gitlab') === 'https://gitlab.com/o/r/-/raw/main/demo.png',
  'an unknown ref falls back to main',
);

assert(
  resolveGitHubReadmeUrl('https://example.com/x.png', 'o', 'r', 'main', 'gitlab') === 'https://example.com/x.png',
  'an absolute URL on another host is left alone',
);
assert(
  resolveGitHubReadmeUrl('data:image/png;base64,AAAA', 'o', 'r', 'main', 'gitlab') === 'data:image/png;base64,AAAA',
  'a data URL is left alone',
);
assert(resolveGitHubReadmeUrl('', 'o', 'r', 'main', 'gitlab') === '', 'an empty href stays empty');

console.log('githubReadme tests passed');
