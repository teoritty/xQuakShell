import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { keepsPaneSelection, ROW_CLASS, type SelectionScopeNode } from './paneSelection';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error(msg);
}

// Minimal DOM stand-in: `el('node-row', parent)` builds the ancestor chain the
// rule walks, without pulling in a DOM implementation (tests here run on tsx).
function el(classes: string[], parent: SelectionScopeNode | null = null): SelectionScopeNode {
  return {
    classList: { contains: (name: string) => classes.includes(name) },
    parentElement: parent,
  };
}

// A pane with one row, and a sibling pane with a row of its own.
const remotePane = el(['file-tree']);
const remoteBody = el(['tree-body'], remotePane);
const remoteRow = el([ROW_CLASS], remoteBody);
const remoteRowLabel = el(['node-name'], remoteRow);
const remoteHeader = el(['panel-header'], remotePane);

const localPane = el(['file-tree']);
const localBody = el(['tree-body'], localPane);
const localRow = el([ROW_CLASS], localBody);

// Clicking a row of this pane — including anything nested in it, which is what
// the event target actually is (the name span, the icon) — keeps the selection.
assert(keepsPaneSelection(remoteRow, remotePane), 'a click on the pane\'s own row keeps its selection');
assert(keepsPaneSelection(remoteRowLabel, remotePane), 'a click on a child of the row keeps the selection');

// The reported bug: empty space in the pane left the highlight on.
assert(!keepsPaneSelection(remoteBody, remotePane), 'empty space inside the pane clears the selection');
assert(!keepsPaneSelection(remoteHeader, remotePane), 'the pane header is not a row and clears the selection');

// Clicking anywhere else in the app clears it too.
assert(!keepsPaneSelection(el(['terminal']), remotePane), 'an unrelated element clears the selection');
assert(!keepsPaneSelection(null, remotePane), 'a missing target clears the selection');

// A row belongs to exactly one pane: the other pane's row must not hold this
// pane's selection open. Both panes render rows with the same class, so the
// containment half of the rule is what separates them.
assert(!keepsPaneSelection(localRow, remotePane), 'the other pane\'s row clears this pane\'s selection');
assert(keepsPaneSelection(localRow, localPane), 'the other pane keeps its own row selected');

// Before the pane is mounted there is no root to compare against; nothing to keep.
assert(!keepsPaneSelection(remoteRow, null), 'an unmounted pane keeps nothing');

// A target detached from the document (e.g. a row removed by a refresh between
// pointerdown and the handler) walks to the top without meeting the pane.
assert(!keepsPaneSelection(el([ROW_CLASS], el(['tree-body'])), remotePane), 'a detached row clears the selection');

// Wiring guard: the rule only fixes the bug if both panes actually listen. A
// pane that stops dismissing on pointerdown reintroduces the sticky highlight,
// and no pure-function test would notice.
const HERE = dirname(fileURLToPath(import.meta.url));
for (const pane of ['FileTree.svelte', 'LocalFileTree.svelte']) {
  const src = readFileSync(join(HERE, pane), 'utf8');
  assert(src.includes('keepsPaneSelection'), `${pane} applies the pane-selection rule`);
  assert(/<svelte:window[^>]*on:pointerdown=/.test(src), `${pane} dismisses selection on window pointerdown`);
}

// The class the rule matches must stay in sync with what the row components render.
for (const node of ['FileTreeNode.svelte', 'LocalFileTreeNode.svelte']) {
  const src = readFileSync(join(HERE, node), 'utf8');
  assert(src.includes(`class="${ROW_CLASS}"`), `${node} still renders rows as .${ROW_CLASS}`);
}

console.log('paneSelection: all assertions passed');
