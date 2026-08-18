// Which repository hosts the plugin UI understands, and how to read a URL on one of them.
//
// It is its own module because two unrelated places need the same answer - the add-repository field
// validates against it, and the README renderer builds raw-file URLs from it - and a second copy of
// the host list is a copy that will disagree with the first the next time a forge is added.

export type Forge = 'github' | 'gitlab';

export const FORGE_BY_HOST: Record<string, Forge> = {
  'github.com': 'github',
  'gitlab.com': 'gitlab',
};

export interface ForgeRepoRef {
  forge: Forge;
  owner: string;
  repo: string;
}

/**
 * Splits a repository URL into the forge that serves it, its namespace and its project.
 *
 * The two forges need different rules. GitHub's namespace is exactly one segment, so anything past
 * owner/repo is a page inside the repository. GitLab's namespace may nest to any depth and is
 * terminated instead by the "/-/" separator GitLab puts before every in-project route - the only
 * thing distinguishing a subgroup from a page.
 */
export function parseForgeRepoRef(repositoryUrl: string): ForgeRepoRef | null {
  try {
    const parsed = new URL(repositoryUrl.trim().replace(/\/+$/, ''));
    const forge = FORGE_BY_HOST[parsed.hostname.toLowerCase()];
    if (!forge) return null;

    const path = parsed.pathname.replace(/\/-\/.*$/, '');
    const parts = path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean);
    if (parts.length < 2) return null;

    const scoped = forge === 'github' ? parts.slice(0, 2) : parts;
    return {
      forge,
      owner: scoped.slice(0, -1).join('/'),
      repo: scoped[scoped.length - 1].replace(/\.git$/, ''),
    };
  } catch {
    return null;
  }
}

/**
 * Reports whether a typed repository URL is one the backend will accept.
 *
 * This mirrors the backend's own validation rather than replacing it: the frontend is not a trust
 * boundary, and this exists so a typo is reported while the user is still looking at the field
 * instead of after a round trip.
 */
export function isSupportedRepositoryURL(value: string): boolean {
  const trimmed = value.trim();
  if (!trimmed) return false;

  let url = trimmed.replace(/\/+$/, '');
  if (!/^https?:\/\//i.test(url)) {
    const named = Object.keys(FORGE_BY_HOST).some(
      (host) => url.toLowerCase() === host || url.toLowerCase().startsWith(`${host}/`),
    );
    url = named ? `https://${url}` : `https://github.com/${url}`;
  }

  try {
    const parsed = new URL(url);
    if (parsed.protocol !== 'https:') return false;
    if (!FORGE_BY_HOST[parsed.hostname.toLowerCase()]) return false;

    // A GitLab namespace may nest, so the segment count is a floor rather than an exact shape.
    // Every segment is shape-checked, where the old GitHub-only rule looked at just the first two.
    const parts = parsed.pathname.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean);
    if (parts.length < 2) return false;
    return parts.every((part) => /^[\w.-]+$/.test(part));
  } catch {
    return false;
  }
}
