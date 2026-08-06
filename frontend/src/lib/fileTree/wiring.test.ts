// frontend/src/lib/fileTree/wiring.test.ts
//
// The modules in this directory are only worth having if both panes actually
// use them. Extracting shared logic and leaving one pane on its own private
// copy is strictly worse than not extracting it: the copy still drifts, and now
// there is a passing test suite next to it suggesting otherwise.
//
// There is no component test infrastructure in this repo — no vitest, no jsdom —
// so this reads the source, the way paneSelection.test.ts and
// discoveryMarkup.test.ts already do.

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}

const LIB = dirname(dirname(fileURLToPath(import.meta.url)));
const PANES = ['FileTree.svelte', 'LocalFileTree.svelte'];
const source = new Map(PANES.map((p) => [p, readFileSync(join(LIB, p), 'utf8')]));

// --- both panes are wired to the shared modules ---
for (const pane of PANES) {
  const src = source.get(pane)!;
  assert(/from '\.\/fileTree\/sorting'/.test(src), `${pane} sorts through fileTree/sorting`);
  assert(/from '\.\/fileTree\/selection'/.test(src), `${pane} selects through fileTree/selection`);
  assert(/from '\.\/fileTree\/paths'/.test(src), `${pane} derives paths through fileTree/paths`);
  assert(/from '\.\/fileTree\/dragPayload'/.test(src), `${pane} reads drags through fileTree/dragPayload`);
  assert(/from '\.\/fileTree\/uniqueName'/.test(src), `${pane} names new entries through fileTree/uniqueName`);
  assert(/from '\.\/fileTree\/deletePrompt'/.test(src), `${pane} words its delete prompt through fileTree/deletePrompt`);
  assert(/from '\.\/fileTree\/columnPrefs'/.test(src), `${pane} persists columns through fileTree/columnPrefs`);
  assert(/from '\.\/fileTree\/FilePaneHeader\.svelte'/.test(src), `${pane} renders the shared header`);
  assert(/import '\.\/fileTree\/fileTreeShared\.css'/.test(src), `${pane} takes its styles from the shared sheet`);
  assert(!/<style>/.test(src), `${pane} must not grow a private style block back`);
}

// --- and no pane keeps a private copy of what it now imports ---
// Each name below was defined identically in both panes before the extraction.
const MUST_NOT_REDEFINE = [
  'formatSize',
  'parseTimestamp',
  'compareValues',
  'sortValue',
  'applySort',
  'reapplySortToTree',
  'findNode',
  'uniqueName',
  'isAtFilesystemRoot',
  'parentDirectory',
  'normalizePathInput',
  'describeDelete',
  'readDragPayload',
  'saveColumnPrefs',
];
for (const pane of PANES) {
  const src = source.get(pane)!;
  for (const name of MUST_NOT_REDEFINE) {
    assert(!new RegExp(`function\\s+${name}\\s*\\(`).test(src), `${pane} must not redefine ${name}`);
  }
}

// --- the selection algebra in particular is gone, not just renamed ---
// This is the shape that indexed the listing without checking the result and
// dereferenced nodes[-1] on a shift-click outside the current directory.
for (const pane of PANES) {
  const src = source.get(pane)!;
  assert(!src.includes('next.add(nodes[i].path)'), `${pane} must not walk a range itself`);
  assert(!/lastSelectedPath\s*!=\s*null\s*\?\s*nodes\.findIndex/.test(src), `${pane} must not resolve the anchor itself`);
}

// --- the delete prompt is computed, not spelled out in the markup ---
for (const pane of PANES) {
  const src = source.get(pane)!;
  assert(src.includes('deletePrompt.requireCheckbox'), `${pane} takes its checkbox rule from describeDelete`);
  assert(!src.includes("'Delete items?'"), `${pane} must not inline the delete copy`);
  assert(!src.includes('This action cannot be undone'), `${pane} must not inline the delete warning`);
}

// --- the path input belongs to the header, not the panes ---
for (const pane of PANES) {
  const src = source.get(pane)!;
  assert(!src.includes('pathInputEl'), `${pane} must not track the path input element itself`);
  assert(!src.includes('class="path-bar"'), `${pane} must not render its own path bar`);
}

// --- the panes stay symmetric ---
// They are near-clones by design (remote over SFTP, local over the host FS).
// Whatever one of them learns to import from here, the other one should too.
const importsOf = (src: string) =>
  [...src.matchAll(/from '\.\/fileTree\/([\w.]+)'/g)].map((m) => m[1]).sort().join(',');
assert(
  importsOf(source.get('FileTree.svelte')!) === importsOf(source.get('LocalFileTree.svelte')!),
  'both panes must import the same set of fileTree modules, or the split has drifted',
);

console.log('OK fileTree/wiring');
