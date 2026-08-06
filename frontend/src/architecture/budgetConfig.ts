// Loader for the shared repo-root code-budgets.json.
//
// The Go gate in test/unit/architecture owns the .go entries and this side owns
// the .ts and .svelte entries, but both read the same limits from the same
// file: a limit that disagrees across languages is a limit nobody trusts.

import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join, posix } from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));

export const SRC_DIR = join(HERE, '..');
export const REPO_ROOT = join(HERE, '..', '..', '..');
export const CONFIG_PATH = join(REPO_ROOT, 'code-budgets.json');

export interface FuncLimit {
  maxCodeLines: number;
  maxParams: number;
  maxNesting: number;
}

export interface BudgetConfig {
  limits: {
    ts: { maxCodeLines: number };
    svelte: { maxScriptCodeLines: number; maxTotalCodeLines: number };
    tsFunc: FuncLimit;
    [key: string]: unknown;
  };
  exemptions: {
    files: { path: string; kind: string; reason: string }[];
    functions: { symbol: string; kind: string; reason: string }[];
  };
  baseline: {
    files: Record<string, { codeLines: number; scriptCodeLines?: number }>;
    functions: Record<string, { codeLines: number; params: number; nesting: number }>;
  };
}

export function loadBudgetConfig(): BudgetConfig {
  return JSON.parse(readFileSync(CONFIG_PATH, 'utf8')) as BudgetConfig;
}

/**
 * Lists the frontend sources that carry a budget, as repo-relative
 * slash-separated paths so the entries read the same on both sides of the
 * config.
 *
 * Test files are excluded: per the size rules, a long test is not a God
 * Object, and capping test length only encourages thinner coverage.
 */
export function listFrontendFiles(): string[] {
  const out: string[] = [];
  walk(SRC_DIR, 'frontend/src', out);
  out.sort();
  return out;
}

function walk(dir: string, relBase: string, out: string[]): void {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const rel = posix.join(relBase, entry.name);
    if (entry.isDirectory()) {
      walk(join(dir, entry.name), rel, out);
      continue;
    }
    if (entry.name.endsWith('.test.ts')) continue;
    if (entry.name.endsWith('.ts') || entry.name.endsWith('.svelte')) out.push(rel);
  }
}

export function readSource(repoRelPath: string): string {
  return readFileSync(join(REPO_ROOT, repoRelPath), 'utf8');
}

export function isFrontendPath(path: string): boolean {
  return path.endsWith('.ts') || path.endsWith('.svelte');
}
