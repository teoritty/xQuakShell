// Source-scanning guard, in the spirit of architecture.test.ts rule 3.
//
// ADR-014's XSS decision is a one-liner with a large blast radius: a discovery
// icon is a file from a plugin's bundle, .svg is an allowed extension, and only
// the extension is checked — the bytes are never validated. Rendered through
// <img src="data:...">, those bytes are an image and nothing in them runs.
// Inserted into the document as markup, they are a script running in the main
// window with the whole app's DOM in reach.
//
// A reviewer cannot keep noticing this forever, so the rule is a test: no
// discovery-related source file may contain {@html}, and PluginIcon.svelte must
// keep rendering through <img>.
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error(msg);
}

const HERE = dirname(fileURLToPath(import.meta.url));
const SRC = join(HERE, '..', '..');

// Every file introduced or touched by the discovery work that can render markup.
const DISCOVERY_FILES = [
  join(HERE, 'PluginIcon.svelte'),
  join(HERE, 'StatusDot.svelte'),
  join(HERE, 'RemoteTreeDiscoveryRow.svelte'),
  join(HERE, 'RemoteTreeConnectionRow.svelte'),
  join(HERE, 'RemoteTreeFolderRow.svelte'),
  join(HERE, 'RemoteTreeNode.svelte'),
  join(HERE, 'RemoteTreeBody.svelte'),
  join(HERE, 'discoveryTree.ts'),
  join(HERE, 'discoverySelection.ts'),
  join(HERE, 'discoveryActions.ts'),
  join(HERE, 'statusDot.ts'),
  join(SRC, 'lib', 'RemoteTree.svelte'),
  join(SRC, 'lib', 'RemoteTreeContextMenu.svelte'),
  join(SRC, 'api', 'discovery.ts'),
  join(SRC, 'stores', 'discoveryState.ts'),
];

const HTML_TAG_RE = /\{@html\b/;

// The comments in these files talk ABOUT {@html} and innerHTML; strip them so
// the prose explaining the rule does not trip the rule.
function stripComments(src: string): string {
  return src
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:])\/\/.*$/gm, '$1');
}

for (const file of DISCOVERY_FILES) {
  const code = stripComments(readFileSync(file, 'utf8'));
  assert(
    !HTML_TAG_RE.test(code),
    `${file} contains {@html}. Plugin-supplied labels and icons must never be inserted as markup — ` +
      `see ADR-014's XSS decision and the comment at the top of PluginIcon.svelte.`
  );
}

// PluginIcon.svelte is the only component that renders a plugin asset, and the
// <img> is the entire reason it exists as a separate component.
{
  const icon = stripComments(readFileSync(join(HERE, 'PluginIcon.svelte'), 'utf8'));
  assert(/<img\b[^>]*\bsrc=\{src\}/.test(icon), 'PluginIcon.svelte must render its data URI through <img src=...>');
  assert(/on:error=/.test(icon), 'PluginIcon.svelte must fall back on error — the icon bytes are never validated');
  assert(
    !/innerHTML|outerHTML|insertAdjacentHTML|document\.write/.test(icon),
    'PluginIcon.svelte must not reach around <img> to insert markup by hand'
  );
}

// StatusDot.svelte must be the ONLY place that draws the dot. Sharing the colour
// function while each row hand-rolls its own markup is how one primitive quietly
// becomes three that drift apart.
{
  const owners: string[] = [];
  for (const file of [...DISCOVERY_FILES, join(HERE, 'RemoteTreeFavorites.svelte')]) {
    if (!file.endsWith('.svelte')) continue;
    if (/class="ping-dot"/.test(stripComments(readFileSync(file, 'utf8')))) owners.push(file);
  }
  assert(
    owners.length === 1 && owners[0].endsWith('StatusDot.svelte'),
    `the status dot must be drawn only by StatusDot.svelte; also drawn by: ${owners.join(', ')}`
  );
}

// The tree source must not reach for the backend's sessionId either: everything
// on this side of the seam is addressed by connectionId. Comments are stripped
// first — several of these files explain in prose why sessionId is absent, and
// that prose is the documentation, not a violation.
for (const file of DISCOVERY_FILES) {
  const code = stripComments(readFileSync(file, 'utf8'));
  assert(
    !/\bsessionId\b/.test(code),
    `${file} mentions sessionId. Discovery is addressed by connectionId on the frontend; ` +
      `sessionId is a backend transport detail resolved in the leading session.`
  );
}

// The row tooltip must name the plugin that drew the row. Two plugins may
// publish a group called "Storage" under one connection, and a tooltip repeating
// the label says nothing the row does not already show. discoveryRowTitle is
// unit-tested next door; what cannot be tested there is that anyone calls it, and
// the component cannot be rendered here (no DOM, no Svelte runtime) — so the link
// is asserted on the source, the technique architecture.test.ts uses.
{
  const row = stripComments(readFileSync(join(HERE, 'RemoteTreeDiscoveryRow.svelte'), 'utf8'));
  assert(
    /discoveryRowTitle\(\s*row\s*,\s*pluginNames\s*\)/.test(row),
    'RemoteTreeDiscoveryRow.svelte must build its tooltip with discoveryRowTitle(row, pluginNames)'
  );
  assert(
    /class="node-name"[^>]*title=\{rowTitle\}/.test(row),
    'the node name must carry that tooltip; title={row.label} repeats what the row already shows'
  );
  // And the map has to arrive: a prop that defaults to an empty Map degrades
  // silently to the plugin id, which is exactly the bug this guards.
  const tree = stripComments(readFileSync(join(SRC, 'lib', 'RemoteTree.svelte'), 'utf8'));
  assert(
    /discoveryPluginNames=\{\$discoveryPluginNames\}/.test(tree),
    'RemoteTree.svelte must pass discoveryPluginNames down to the tree body'
  );
}

console.log('discoveryMarkup.test.ts: all passed');
