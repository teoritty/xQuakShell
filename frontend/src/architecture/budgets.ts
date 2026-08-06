// Frontend file size budgets: the same limit / exemption / ratchet mechanics
// the Go gate applies, over .ts and .svelte.
//
// A Svelte component carries two numbers. Markup and CSS inflate the total
// without being the part that rots, so the script block - where logic
// accumulates - carries the tighter limit, and the total only catches a
// component that has grown into a page.

import type { BudgetConfig } from './budgetConfig';
import { isFrontendPath, listFrontendFiles, readSource } from './budgetConfig';
import type { FileMeasurement } from './measure';
import { measureSource } from './measure';

export function measureFrontendFiles(): Map<string, FileMeasurement> {
  const out = new Map<string, FileMeasurement>();
  for (const rel of listFrontendFiles()) {
    out.set(rel, measureSource(rel, readSource(rel)));
  }
  return out;
}

interface Budget {
  /** The measured numbers, paired with the limit each must meet. */
  parts: { label: string; value: number; limit: number }[];
}

function budgetFor(rel: string, m: FileMeasurement, cfg: BudgetConfig): Budget {
  if (!rel.endsWith('.svelte')) {
    return { parts: [{ label: 'code lines', value: m.codeLines, limit: cfg.limits.ts.maxCodeLines }] };
  }
  return {
    parts: [
      { label: 'script code lines', value: m.scriptCodeLines ?? 0, limit: cfg.limits.svelte.maxScriptCodeLines },
      { label: 'total code lines', value: m.codeLines, limit: cfg.limits.svelte.maxTotalCodeLines },
    ],
  };
}

const over = (b: Budget): boolean => b.parts.some((p) => p.value > p.limit);
const describe = (b: Budget): string => b.parts.map((p) => `${p.value} ${p.label} (limit ${p.limit})`).join(', ');

function grew(m: FileMeasurement, was: { codeLines: number; scriptCodeLines?: number }): boolean {
  return m.codeLines > was.codeLines || (m.scriptCodeLines ?? 0) > (was.scriptCodeLines ?? 0);
}

function shrank(m: FileMeasurement, was: { codeLines: number; scriptCodeLines?: number }): boolean {
  return m.codeLines !== was.codeLines || (m.scriptCodeLines ?? 0) !== (was.scriptCodeLines ?? 0);
}

/** Returns one message per violation; an empty array means the gate passes. */
export function checkFrontendFileBudgets(cfg: BudgetConfig): string[] {
  const measured = measureFrontendFiles();
  const exempt = new Set(cfg.exemptions.files.filter((e) => isFrontendPath(e.path)).map((e) => e.path));
  const issues: string[] = [];

  for (const [rel, m] of measured) {
    const budget = budgetFor(rel, m, cfg);
    const recorded = cfg.baseline.files[rel];

    if (exempt.has(rel)) {
      if (!over(budget)) {
        issues.push(`${rel}: ${describe(budget)} is within budget, so its exemption in code-budgets.json is stale; delete the entry`);
      }
      continue;
    }

    if (!recorded) {
      if (over(budget)) {
        issues.push(
          `${rel}: ${describe(budget)}. Split it into a container plus presentational subcomponents, ` +
            `or - if it is generated or a declarations-only surface - add an exemption with a kind and a reason to code-budgets.json`
        );
      }
      continue;
    }

    if (grew(m, recorded)) {
      issues.push(
        `${rel}: grew to ${describe(budget)} from the recorded ${recorded.codeLines} total / ` +
          `${recorded.scriptCodeLines ?? 0} script. Baselined files may shrink, never grow`
      );
    } else if (!over(budget)) {
      issues.push(`${rel}: ${describe(budget)} now meets the budget; drop it from the baseline (npm run budgets:update)`);
    } else if (shrank(m, recorded)) {
      issues.push(`${rel}: shrank to ${describe(budget)}; re-record it so the ratchet tightens (npm run budgets:update)`);
    }
  }

  issues.push(...staleEntries(cfg, measured));
  return issues;
}

function staleEntries(cfg: BudgetConfig, measured: Map<string, FileMeasurement>): string[] {
  const issues: string[] = [];
  for (const path of Object.keys(cfg.baseline.files)) {
    if (isFrontendPath(path) && !measured.has(path)) {
      issues.push(`${path}: is baselined in code-budgets.json but is no longer a budgeted frontend file; remove the entry`);
    }
  }
  for (const e of cfg.exemptions.files) {
    if (isFrontendPath(e.path) && !measured.has(e.path)) {
      issues.push(`${e.path}: is exempted in code-budgets.json but is no longer a budgeted frontend file; remove the entry`);
    }
  }
  return issues;
}
