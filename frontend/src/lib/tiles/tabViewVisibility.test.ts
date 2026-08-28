// Every per-tab view must hide itself when it is not the active tab.
//
// TileGroup renders ALL of a tile's tabs at once and passes each one an `active` prop; nothing
// above them unmounts the inactive ones. So a view that ignores `active` is drawn on top of its
// siblings, and a tile with two tabs splits the screen between them instead of showing one.
//
// This is a source-level check because the frontend suite has no DOM: the contract lives in a
// Svelte template and its scoped CSS, and reading them is the only way to assert it. It is worth
// asserting anyway - the failure is silent, looks like a layout bug rather than a missing prop,
// and the obvious way to write a new tab view is to copy a sibling that already got it wrong.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const libDir = join(process.cwd(), 'src', 'lib');

/** Every component TileGroup renders once per tab. */
const tabViews = ['SessionView.svelte', 'SurfaceView.svelte', 'LocalTerminalView.svelte'];

for (const name of tabViews) {
  const src = readFileSync(join(libDir, name), 'utf8');

  assert(
    /export let active/.test(src),
    `${name} does not take an \`active\` prop; TileGroup passes one to every tab view`,
  );

  assert(
    /class:visible=\{active\}/.test(src),
    `${name} never binds its visibility to \`active\`. TileGroup draws every tab of a tile at ` +
      `once, so without this the inactive tabs render on top of the active one and the tile ` +
      `looks like a broken split.`,
  );

  assert(
    /display:\s*none/.test(src),
    `${name} has no hidden default. \`class:visible\` only helps if the base rule hides it.`,
  );
}

console.log('tabViewVisibility.test passed');
