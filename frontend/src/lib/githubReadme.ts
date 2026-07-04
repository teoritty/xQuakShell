import DOMPurify from 'dompurify';
import { Marked } from 'marked';

export interface GitHubRepoRef {
  owner: string;
  repo: string;
}

export function parseGitHubRepoRef(repositoryUrl: string): GitHubRepoRef | null {
  try {
    const parsed = new URL(repositoryUrl.trim().replace(/\/+$/, ''));
    const parts = parsed.pathname.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean);
    if (parts.length < 2) return null;
    return { owner: parts[0], repo: parts[1] };
  } catch {
    return null;
  }
}

export function resolveGitHubReadmeUrl(
  href: string | null | undefined,
  owner: string,
  repo: string,
  ref: string,
): string {
  if (!href) return '';
  const trimmed = href.trim();
  if (!trimmed) return '';
  if (/^data:/i.test(trimmed)) return trimmed;
  if (/^https?:\/\//i.test(trimmed)) {
    const blobMatch = trimmed.match(/^https?:\/\/github\.com\/[^/]+\/[^/]+\/blob\/([^/]+)\/(.+)$/i);
    if (blobMatch) {
      return `https://raw.githubusercontent.com/${owner}/${repo}/${blobMatch[1]}/${blobMatch[2]}`;
    }
    return trimmed;
  }
  if (trimmed.startsWith('//')) return `https:${trimmed}`;

  const branch = ref || 'main';
  const relativePath = trimmed.replace(/^\.\//, '').replace(/^\//, '');
  return `https://raw.githubusercontent.com/${owner}/${repo}/${branch}/${relativePath}`;
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function createMarkedParser(owner: string, repo: string, ref: string): Marked {
  const branch = ref || 'main';

  return new Marked({
    gfm: true,
    breaks: true,
    renderer: {
      image({ href, title, text }) {
        const src = owner && repo ? resolveGitHubReadmeUrl(href, owner, repo, branch) : (href ?? '');
        const titleAttr = title ? ` title="${escapeHtml(title)}"` : '';
        return `<img src="${escapeHtml(src)}" alt="${escapeHtml(text ?? '')}"${titleAttr} loading="lazy" />`;
      },
      link({ href, title, tokens }) {
        const resolved = href && owner && repo ? resolveGitHubReadmeUrl(href, owner, repo, branch) : (href ?? '');
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
  const parser = createMarkedParser(owner, repo, ref);
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
