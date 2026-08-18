import { isSupportedRepositoryURL } from './forgeRepo';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

function accepts(url: string, msg: string) {
  assert(isSupportedRepositoryURL(url), `${msg}: ${url} was rejected`);
}

function rejects(url: string, msg: string) {
  assert(!isSupportedRepositoryURL(url), `${msg}: ${url} was accepted`);
}

accepts('https://github.com/teoritty/xQuakShell', 'a github.com URL');
accepts('https://gitlab.com/teoritty/xQuakShell', 'a gitlab.com URL');

// A GitLab namespace nests. The GitHub-only rule this replaced checked exactly two segments, so a
// perfectly valid subgroup project was reported as a malformed URL and could not be added at all.
accepts('https://gitlab.com/group/subgroup/plugin', 'a GitLab subgroup project');

accepts('teoritty/xQuakShell', 'a bare pair, which means the default forge');
accepts('gitlab.com/teoritty/xQuakShell', 'a bare gitlab path, which names its own host');
accepts('GitLab.com/teoritty/xQuakShell', 'host matching is case-insensitive');

rejects('https://bitbucket.org/team/plugin', 'an unsupported host');
rejects('http://gitlab.com/teoritty/xQuakShell', 'plain http, even on a supported host');
rejects('https://gitlab.com.evil.example/team/plugin', 'a lookalike host');
rejects('https://gitlab.com/teoritty', 'a namespace with no project');
rejects('https://gitlab.com/group/pl ugin', 'a segment that is not a path component');
rejects('', 'an empty value');
rejects('   ', 'whitespace only');

console.log('forgeRepo tests passed');
