// Frontend function budgets: the limit / exemption / ratchet mechanics of the
// file budgets, applied to size, parameter count and nesting depth.

import type { BudgetConfig, FuncLimit } from './budgetConfig';
import { listFrontendFiles, readSource } from './budgetConfig';
import type { FuncShape } from './funcShapes';
import { measureFuncs } from './funcShapes';

export function measureFrontendFuncs(): Map<string, FuncShape> {
  const out = new Map<string, FuncShape>();
  for (const rel of listFrontendFiles()) {
    for (const [symbol, shape] of measureFuncs(rel, readSource(rel))) {
      out.set(symbol, shape);
    }
  }
  return out;
}

export function exceeds(shape: FuncShape, limit: FuncLimit): boolean {
  return (
    shape.codeLines > limit.maxCodeLines ||
    shape.params > limit.maxParams ||
    shape.nesting > limit.maxNesting
  );
}

const describe = (s: FuncShape): string =>
  `${s.codeLines} code lines / ${s.params} params / nesting ${s.nesting}`;

const grew = (s: FuncShape, was: FuncShape): boolean =>
  s.codeLines > was.codeLines || s.params > was.params || s.nesting > was.nesting;

const differs = (s: FuncShape, was: FuncShape): boolean =>
  s.codeLines !== was.codeLines || s.params !== was.params || s.nesting !== was.nesting;

/** Returns one message per violation; an empty array means the gate passes. */
export function checkFrontendFuncBudgets(cfg: BudgetConfig): string[] {
  const measured = measureFrontendFuncs();
  const limit = cfg.limits.tsFunc;
  const exempt = new Set(cfg.exemptions.functions.map((e) => e.symbol));
  const issues: string[] = [];

  for (const [symbol, shape] of measured) {
    const recorded = cfg.baseline.functions[symbol];

    if (exempt.has(symbol)) {
      if (!exceeds(shape, limit)) {
        issues.push(`${symbol}: ${describe(shape)} is within every limit, so its exemption is stale; delete the entry`);
      }
      continue;
    }

    if (!recorded) {
      if (exceeds(shape, limit)) {
        issues.push(
          `${symbol}: ${describe(shape)} exceeds ${limit.maxCodeLines} code lines / ${limit.maxParams} params / ` +
            `nesting ${limit.maxNesting}; extract a helper, or pass an options object instead of a long parameter list`
        );
      }
      continue;
    }

    if (grew(shape, recorded)) {
      issues.push(`${symbol}: grew from ${describe(recorded)} to ${describe(shape)}. Baselined functions may shrink, never grow`);
    } else if (!exceeds(shape, limit)) {
      issues.push(`${symbol}: is down to ${describe(shape)} and now meets every limit; drop it from the baseline (npm run budgets:update)`);
    } else if (differs(shape, recorded)) {
      issues.push(`${symbol}: shrank from ${describe(recorded)} to ${describe(shape)}; re-record it so the ratchet tightens (npm run budgets:update)`);
    }
  }

  return issues.concat(staleFuncEntries(cfg, measured));
}

/**
 * Only frontend symbols are checked here. The two halves of the config are
 * told apart by the file part of the symbol, so a Go entry never looks stale
 * to this side and vice versa.
 */
function staleFuncEntries(cfg: BudgetConfig, measured: Map<string, FuncShape>): string[] {
  const isFrontend = (symbol: string): boolean => symbol.startsWith('frontend/');
  const issues: string[] = [];
  for (const symbol of Object.keys(cfg.baseline.functions)) {
    if (isFrontend(symbol) && !measured.has(symbol)) {
      issues.push(`${symbol}: is baselined but no longer exists (deleted or renamed); remove the entry`);
    }
  }
  for (const e of cfg.exemptions.functions) {
    if (isFrontend(e.symbol) && !measured.has(e.symbol)) {
      issues.push(`${e.symbol}: is exempted but no longer exists (deleted or renamed); remove the entry`);
    }
  }
  return issues;
}
