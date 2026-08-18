import DOMPurify from 'dompurify';
import { Marked } from 'marked';

import { parseForgeRepoRef, type Forge, type ForgeRepoRef } from './forgeRepo';

export type ReadmeForge = Forge;
export type GitHubRepoRef = ForgeRepoRef;

export function parseGitHubRepoRef(repositoryUrl: string): GitHubRepoRef | null {
  return parseForgeRepoRef(repositoryUrl);
}

// rawFileUrl builds the URL that serves a repository file as bytes. The two forges have nothing in
// common here: GitHub serves raw files from a separate host, GitLab from a "/-/raw/" route on the
// repository's own URL.
function rawFileUrl(forge: ReadmeForge, owner: string, repo: string, ref: string, path: string): string {
  if (forge === 'gitlab') {
    return `https://gitlab.com/${owner}/${repo}/-/raw/${ref}/${path}`;
  }
  return `https://raw.githubusercontent.com/${owner}/${repo}/${ref}/${path}`;
}

// BLOB_PATTERNS matches a link to a file's rendered page, which a README author writes because it
// is what the browser gives them when they copy the address. Left alone, an <img> pointing at it
// renders the HTML page instead of the image.
const BLOB_PATTERNS: Record<ReadmeForge, RegExp> = {
  github: /^https?:\/\/github\.com\/[^/]+\/[^/]+\/blob\/([^/]+)\/(.+)$/i,
  gitlab: /^https?:\/\/gitlab\.com\/(?:.+?)\/-\/blob\/([^/]+)\/(.+)$/i,
};

export function resolveGitHubReadmeUrl(
  href: string | null | undefined,
  owner: string,
  repo: string,
  ref: string,
  forge: ReadmeForge = 'github',
): string {
  if (!href) return '';
  const trimmed = href.trim();
  if (!trimmed) return '';
  if (/^data:/i.test(trimmed)) return trimmed;
  if (/^https?:\/\//i.test(trimmed)) {
    const blobMatch = trimmed.match(BLOB_PATTERNS[forge]);
    if (blobMatch) {
      return rawFileUrl(forge, owner, repo, blobMatch[1], blobMatch[2]);
    }
    return trimmed;
  }
  if (trimmed.startsWith('//')) return `https:${trimmed}`;

  const branch = ref || 'main';
  const relativePath = trimmed.replace(/^\.\//, '').replace(/^\//, '');
  return rawFileUrl(forge, owner, repo, branch, relativePath);
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function createMarkedParser(owner: string, repo: string, ref: string, forge: ReadmeForge): Marked {
  const branch = ref || 'main';

  return new Marked({
    gfm: true,
    breaks: true,
    renderer: {
      image({ href, title, text }) {
        const src = owner && repo ? resolveGitHubReadmeUrl(href, owner, repo, branch, forge) : (href ?? '');
        const titleAttr = title ? ` title="${escapeHtml(title)}"` : '';
        return `<img src="${escapeHtml(src)}" alt="${escapeHtml(text ?? '')}"${titleAttr} loading="lazy" />`;
      },
      link({ href, title, tokens }) {
        const resolved = href && owner && repo ? resolveGitHubReadmeUrl(href, owner, repo, branch, forge) : (href ?? '');
        const text = this.parser.parseInline(tokens);
        const titleAttr = title ? ` title="${escapeHtml(title)}"` : '';
        return `<a href="${escapeHtml(resolved || '#')}" target="_blank" rel="noopener noreferrer"${titleAttr}>${text}</a>`;
      },
    },
  });
}

export function renderGitHubReadme(markdown: string, repositoryUrl: string, ref: string): string {
  if (!markdown?.trim()) return '';

  const repoRef = parseGitHubRepoRef(repositoryUrl);
  const owner = repoRef?.owner ?? '';
  const repo = repoRef?.repo ?? '';
  const parser = createMarkedParser(owner, repo, ref, repoRef?.forge ?? 'github');
  const raw = parser.parse(markdown, { async: false }) as string;

  return DOMPurify.sanitize(raw, {
    ADD_ATTR: ['target', 'rel', 'loading'],
  });
}

export function formatPublishedDate(iso: string): string {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}
