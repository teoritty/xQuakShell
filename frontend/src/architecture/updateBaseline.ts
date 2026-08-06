// Re-records the frontend half of the size baseline in code-budgets.json.
//
// Run with `npm run budgets:update`. The Go half is re-recorded by
// `go run ./scripts/budgets -update`; each side rewrites only the entries for
// its own file extensions, and both emit keys in sorted order so the two
// writers never fight over the layout.
//
// Limits and exemptions are never touched here. Those are human decisions;
// only the debt numbers are machine-owned.

import { writeFileSync } from 'node:fs';
import { CONFIG_PATH, isFrontendPath, loadBudgetConfig } from './budgetConfig';
import { measureFrontendFiles } from './budgets';
import { exceeds, measureFrontendFuncs } from './funcBudgets';

const cfg = loadBudgetConfig();
const measured = measureFrontendFiles();
const exempt = new Set(cfg.exemptions.files.map((e) => e.path));

const next: Record<string, { codeLines: number; scriptCodeLines?: number }> = {};
for (const [path, entry] of Object.entries(cfg.baseline.files)) {
  if (!isFrontendPath(path)) next[path] = entry;
}

const added: string[] = [];
for (const [rel, m] of measured) {
  if (exempt.has(rel)) continue;
  const overScript =
    rel.endsWith('.svelte') && (m.scriptCodeLines ?? 0) > cfg.limits.svelte.maxScriptCodeLines;
  const overTotal = rel.endsWith('.svelte')
    ? m.codeLines > cfg.limits.svelte.maxTotalCodeLines
    : m.codeLines > cfg.limits.ts.maxCodeLines;
  if (!overScript && !overTotal) continue;

  if (!cfg.baseline.files[rel]) added.push(rel);
  next[rel] = rel.endsWith('.svelte')
    ? { codeLines: m.codeLines, scriptCodeLines: m.scriptCodeLines }
    : { codeLines: m.codeLines };
}

cfg.baseline.files = sortKeys(next);

const nextFuncs: Record<string, { codeLines: number; params: number; nesting: number }> = {};
for (const [symbol, entry] of Object.entries(cfg.baseline.functions)) {
  if (!symbol.startsWith('frontend/')) nextFuncs[symbol] = entry;
}
const exemptFuncs = new Set(cfg.exemptions.functions.map((e) => e.symbol));
for (const [symbol, shape] of measureFrontendFuncs()) {
  if (exemptFuncs.has(symbol) || !exceeds(shape, cfg.limits.tsFunc)) continue;
  if (!cfg.baseline.functions[symbol]) added.push(symbol);
  nextFuncs[symbol] = shape;
}
cfg.baseline.functions = sortKeys(nextFuncs);
writeFileSync(CONFIG_PATH, JSON.stringify(cfg, null, 2) + '\n', 'utf8');

for (const rel of added.sort()) {
  console.error(`WARNING: added ${rel} to the baseline. New debt should be paid, not recorded.`);
}
console.log(
  `budgets: ${Object.keys(cfg.baseline.files).length} files and ` +
    `${Object.keys(cfg.baseline.functions).length} functions baselined`
);

function sortKeys<T>(obj: Record<string, T>): Record<string, T> {
  const out: Record<string, T> = {};
  for (const key of Object.keys(obj).sort()) out[key] = obj[key];
  return out;
}
