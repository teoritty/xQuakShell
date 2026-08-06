// Layer-boundary guard test.
//
// Mirrors the backend's architecture test: enforces the frontend's api/actions/backend
// layering going forward, while explicitly allowlisting the small, already-reviewed set
// of exceptions that exist today. Each exception below was investigated (not guessed)
// and is documented inline in its source file. The
// allowlists are a closed, deliberate list — extending them requires the same scrutiny
// as the original findings, not a rubber stamp.
//
// Rules enforced:
//  1. api/*.ts must not import VALUE bindings from ../stores/appState (type-only is fine),
//     except the specific documented cases below (error reporting / host-key reset that
//     don't fit callBackend's four error shapes).
//  2. actions/*.ts must not import `getGateway` directly, except the specific files that
//     use it as the established missing-gateway pre-flight guard idiom,
//     reproducing exact original stores/api.ts fallback behavior.
//  3. Nothing outside a small set of sanctioned files may reference the raw Wails bridge
//     (`window.go` / `window.runtime` / `(window as any).go` / `(window as any).runtime`).
//     Scoped to *.ts modules under src (api/actions/backend/stores/lib layers) — this
//     mirrors the plan's "God-component splits ... out of scope" carve-out: pre-existing
//     .svelte components that touch the bridge directly (Terminal.svelte, App.svelte,
//     SettingsDialog.svelte, KnownHostsManager.svelte, LogViewerApp.svelte) are a known,
//     separate, deferred cleanup area, not part of this refactor's api/actions/backend
//     gateway boundary.

import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { stripComments } from './architecture/measure';
import { loadBudgetConfig } from './architecture/budgetConfig';
import { checkFrontendFileBudgets } from './architecture/budgets';
import { checkFrontendFuncBudgets } from './architecture/funcBudgets';
import { checkComments } from './architecture/comments';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error(msg);
}

const SRC_DIR = dirname(fileURLToPath(import.meta.url));

// ---------------------------------------------------------------------------
// Rule 1: api/*.ts value imports from ../stores/appState
// ---------------------------------------------------------------------------
//
// Closed allowlist: filename -> set of specific import names allowed as VALUE
// imports from '../stores/appState'. Anything not listed here (a new import name,
// or the same name in a file not listed) fails the test. `import type {...}` is
// always allowed and is not tracked here.
//
// - showError: these five files bypass callBackend's four error shapes to report
//   errors directly (documented in each file's header comment).
// - pendingHostKey (sessions.ts only): reproduces exact original host-key-reset
//   behavior (pendingHostKey.set(null) in resolveHostKeyRpc); a writable store,
//   not a type.
const API_APPSTATE_VALUE_IMPORT_ALLOWLIST: Map<string, Set<string>> = new Map([
  ['audit.ts', new Set(['showError'])],
  ['githubPlugins.ts', new Set(['showError'])],
  ['localFs.ts', new Set(['showError'])],
  ['plugins.ts', new Set(['showError'])],
  ['pluginRuntime.ts', new Set(['showError'])],
  ['sessions.ts', new Set(['showError', 'pendingHostKey'])],
]);

// ---------------------------------------------------------------------------
// Rule 2: actions/*.ts direct getGateway usage
// ---------------------------------------------------------------------------
//
// Closed allowlist: these five files import `getGateway` directly from
// '../backend/context' and call it as an explicit pre-flight guard before
// invoking the atomic api/* RPC wrapper. This is the established
// missing-gateway-guard idiom: the atomic api/* wrappers
// return their fallback *before* the caller's try/catch runs any dependent
// refresh logic, so without this explicit guard the surrounding orchestration
// would proceed as if the mutation succeeded even when there is no backend.
// settingsActions.ts is deliberately NOT in this list: its wrapped
// getSettings/saveSettings already perform their own getGateway() check
// internally and it needs no separate guard.
const ACTIONS_GET_GATEWAY_ALLOWLIST: Set<string> = new Set([
  'connectionActions.ts',
  'folderActions.ts',
  'sessionActions.ts',
  'vaultActions.ts',
  'protocolActions.ts',
]);

// ---------------------------------------------------------------------------
// Rule 3: raw Wails bridge access (window.go / window.runtime)
// ---------------------------------------------------------------------------
//
// Closed allowlist of *.ts files (relative to src/) permitted to touch the raw
// bridge object directly. Everything else must go through backend/liveGateway.ts.
//
// - backend/liveGateway.ts: the canonical, sole place the AppGateway RPC surface
//   is meant to be constructed from window.go/window.runtime.
// - lib/osFileDrop.ts: a second, distinct Wails bridge (native OS file-drop
//   *events*, not AppGateway RPC calls). This repo's own persisted engineering
//   notes (memory: "OS file-drop architecture") document this as a deliberate,
//   standing decision, not an oversight to route through liveGateway.ts.
// - stores/pluginState.ts: same category as osFileDrop.ts - a third, distinct
//   Wails bridge (plugin runtime EventsOn subscriptions: PluginContributionsChanged
//   / PluginStateChanged / PluginViewMessage), not an AppGateway RPC call. Found
//   during this task's self-review re-grep (the prior narrower regex scan missed
//   it because it uses the `(window as any).runtime` spelling, not literal
//   `window.runtime`); confirmed pre-existing and unrelated to the api/actions
//   atomization this guard protects.
const WINDOW_BRIDGE_ALLOWLIST: Set<string> = new Set([
  join('backend', 'liveGateway.ts'),
  join('lib', 'osFileDrop.ts'),
  join('stores', 'pluginState.ts'),
]);

const WINDOW_BRIDGE_RE = /\(window\s+as\s+any\)\s*\.\s*(go|runtime)\b|\bwindow\.(go|runtime)\b/;

function listTsFiles(dir: string): string[] {
  return readdirSync(dir).filter((f) => f.endsWith('.ts') && !f.endsWith('.test.ts'));
}

function walkTsFiles(dir: string, base: string, out: string[]): void {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    const rel = join(base, entry.name);
    if (entry.isDirectory()) {
      walkTsFiles(full, rel, out);
    } else if (entry.name.endsWith('.ts') && !entry.name.endsWith('.test.ts')) {
      out.push(rel);
    }
  }
}

// --- Rule 1 ---
function checkApiAppStateImports(): void {
  const dir = join(SRC_DIR, 'api');
  for (const file of listTsFiles(dir)) {
    const src = readFileSync(join(dir, file), 'utf8');
    const importRe = /^import\s+(type\s+)?\{([^}]*)\}\s+from\s+'\.\.\/stores\/appState';?\s*$/gm;
    let match: RegExpExecArray | null;
    while ((match = importRe.exec(src))) {
      const isTypeOnly = !!match[1];
      if (isTypeOnly) continue; // `import type {...}` always allowed
      const names = match[2]
        .split(',')
        .map((n) => n.trim())
        .filter(Boolean);
      const allowed = API_APPSTATE_VALUE_IMPORT_ALLOWLIST.get(file) ?? new Set<string>();
      for (const name of names) {
        assert(
          allowed.has(name),
          `api/${file} imports value '${name}' from ../stores/appState, which is not in ` +
            `the reviewed allowlist. api/* modules must not depend on stores/appState value ` +
            `bindings except the documented, allowlisted exceptions.`
        );
      }
    }
  }
}

// --- Rule 2 ---
function checkActionsGetGateway(): void {
  const dir = join(SRC_DIR, 'actions');
  for (const file of listTsFiles(dir)) {
    const src = readFileSync(join(dir, file), 'utf8');
    const importsGetGateway = /^import\s*\{[^}]*\bgetGateway\b[^}]*\}\s*from\s*'\.\.\/backend\/context';?\s*$/m.test(
      src
    );
    if (importsGetGateway) {
      assert(
        ACTIONS_GET_GATEWAY_ALLOWLIST.has(file),
        `actions/${file} imports getGateway directly, which is only permitted for the ` +
          `established missing-gateway pre-flight guard idiom in the ` +
          `reviewed allowlist. actions/* must otherwise reach the gateway only through api/*.`
      );
    }
    assert(
      !/\bwindow\.(go|runtime)\b|\(window\s+as\s+any\)/.test(stripComments(src)),
      `actions/${file} references window directly; actions/* must only reach the backend ` +
        `through api/* (or the documented getGateway guard idiom).`
    );
  }
}

// --- Rule 3 ---
function checkWindowBridgeUsage(): void {
  const files: string[] = [];
  walkTsFiles(SRC_DIR, '', files);
  for (const rel of files) {
    const src = readFileSync(join(SRC_DIR, rel), 'utf8');
    const stripped = stripComments(src);
    if (WINDOW_BRIDGE_RE.test(stripped)) {
      assert(
        WINDOW_BRIDGE_ALLOWLIST.has(rel),
        `${rel} references the raw Wails bridge (window.go/window.runtime), which is only ` +
          `permitted in the reviewed allowlist (backend/liveGateway.ts, lib/osFileDrop.ts, ` +
          `stores/pluginState.ts). All other code must go through backend/liveGateway.ts.`
      );
    }
  }
  // Sanity check: every allowlisted file actually exists and actually matches, so the
  // allowlist can't silently drift into dead entries that hide a real regression.
  for (const rel of WINDOW_BRIDGE_ALLOWLIST) {
    const src = readFileSync(join(SRC_DIR, rel), 'utf8');
    assert(
      WINDOW_BRIDGE_RE.test(stripComments(src)),
      `${rel} is allowlisted for window bridge access but no longer references it; remove ` +
        `the stale allowlist entry.`
    );
  }
}

// --- Rule 4 ---
function checkFileBudgets(): void {
  const issues = checkFrontendFileBudgets(loadBudgetConfig());
  assert(issues.length === 0, ['size budgets:', ...issues].join('\n  '));
}

// --- Rule 6 ---
function checkCommentRules(): void {
  const issues = checkComments();
  assert(issues.length === 0, ['comment rules:', ...issues].join('\n  '));
}

// --- Rule 5 ---
function checkFuncBudgets(): void {
  const issues = checkFrontendFuncBudgets(loadBudgetConfig());
  assert(issues.length === 0, ['function budgets:', ...issues].join('\n  '));
}

checkApiAppStateImports();
checkActionsGetGateway();
checkWindowBridgeUsage();
checkFileBudgets();
checkFuncBudgets();
checkCommentRules();

console.log('architecture.test passed');
