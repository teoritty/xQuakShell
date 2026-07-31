import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { get } from 'svelte/store';
import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { buildTree, flattenTree } from '../lib/remoteTree/buildTree';
import { emptyDiscoverySelection } from '../lib/remoteTree/discoverySelection';
import { discoveryKey } from '../lib/remoteTree/types';
import type { Connection, Folder } from './appState';
import {
  discoveryExpanded,
  discoveryIconKey,
  discoveryIcons,
  discoverySelection,
  discoverySnapshots,
  forgetDiscoveryTree,
  forgetUnavailableDiscovery,
  isDiscoveryRootExpanded,
  onDiscoveryTreeChanged,
  refreshDiscoveryIcons,
  setDiscoveryNodeExpanded,
  toggleDiscoveryRoot,
} from './discoveryState';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const folders: Folder[] = [];
const connections: Connection[] = [
  { id: 'c1', name: 'web', host: 'h1', port: 22, folderId: '', order: 0, users: [], defaultUserId: '' },
];

const snapshot = {
  connectionId: 'c1',
  plugins: [
    {
      pluginId: 'p1',
      nodes: [
        { id: 'containers', parentId: '', kind: 'group' as const, label: 'Containers', order: 0, actions: [] },
      ],
      branches: { '': { state: 'ready' as const } },
    },
  ],
};

/** The rows the tree would actually render, given the store's current state. */
function renderedDiscoveryRows(): string[] {
  return flattenTree(
    buildTree(folders, connections, new Set(), '', {
      snapshots: get(discoverySnapshots),
      expanded: get(discoveryExpanded),
    })
  )
    .filter((n) => n.type === 'discovery')
    .map((n) => n.name);
}

function reset() {
  discoverySnapshots.set(new Map());
  discoveryExpanded.set(new Map());
  discoverySelection.set(emptyDiscoverySelection());
  discoveryIcons.set(new Map());
}

async function run() {
  const fake = createFakeGateway();
  setGateway(fake);
  fake.program('GetDiscoveryTree', snapshot);
  fake.program('SetDiscoveryObserved', undefined);

  // --- expanding publishes the FULL observe set and fetches the tree ---
  reset();
  await toggleDiscoveryRoot('c1');
  assert(isDiscoveryRootExpanded(get(discoveryExpanded), 'c1'), 'the root is expanded');
  let observe = fake.calls.filter((c) => c.method === 'SetDiscoveryObserved').pop();
  assert(JSON.stringify(observe!.args[1]) === JSON.stringify(['']), 'expanding observes the root');
  assert(renderedDiscoveryRows().join(',') === 'Containers', 'the subtree renders');

  await setDiscoveryNodeExpanded('c1', discoveryKey('p1', 'containers'), true);
  observe = fake.calls.filter((c) => c.method === 'SetDiscoveryObserved').pop();
  assert(
    JSON.stringify((observe!.args[1] as string[]).slice().sort()) === JSON.stringify(['', 'containers']),
    'a nested expansion resends the WHOLE set, never a delta'
  );

  // --- THE REGRESSION: the last ready session closes ---
  //
  // The connection row stops drawing its expander at that moment, so if the
  // expansion survived, the rows would stay on screen with nothing left that can
  // collapse them. Deleting the wiring in RemoteTree.svelte must fail here.
  forgetUnavailableDiscovery(new Set());
  assert(
    renderedDiscoveryRows().length === 0,
    'closing the last ready session must remove the expanded subtree from the tree'
  );
  assert(!isDiscoveryRootExpanded(get(discoveryExpanded), 'c1'), 'and drop the expansion itself');
  assert(get(discoverySnapshots).size === 0, 'and the snapshot, which is never cached or persisted');

  // --- a connection that still has a ready session is untouched ---
  reset();
  await toggleDiscoveryRoot('c1');
  forgetUnavailableDiscovery(new Set(['c1', 'c2']));
  assert(renderedDiscoveryRows().join(',') === 'Containers', 'a live connection keeps its subtree');

  // --- teardown clears the selection too, which closes the action menu ---
  reset();
  await toggleDiscoveryRoot('c1');
  discoverySelection.set({
    connectionId: 'c1',
    pluginId: 'p1',
    parentKey: discoveryKey('p1', ''),
    keys: new Set([discoveryKey('p1', 'containers')]),
    lastKey: discoveryKey('p1', 'containers'),
  });
  forgetUnavailableDiscovery(new Set());
  assert(get(discoverySelection).keys.size === 0, 'the selection cannot outlive the rows it points at');

  // --- a snapshot without an expansion is cleaned up as well ---
  reset();
  discoverySnapshots.set(new Map([['c1', snapshot]]));
  forgetUnavailableDiscovery(new Set());
  assert(get(discoverySnapshots).size === 0, 'an orphaned snapshot is not leaked');

  // --- collapsing drops the snapshot and observes nothing ---
  reset();
  await toggleDiscoveryRoot('c1');
  await toggleDiscoveryRoot('c1');
  observe = fake.calls.filter((c) => c.method === 'SetDiscoveryObserved').pop();
  assert(JSON.stringify(observe!.args[1]) === JSON.stringify([]), 'a collapsed connection observes nothing');
  assert(get(discoverySnapshots).size === 0, 'collapsing forgets the snapshot rather than let it go stale');
  assert(renderedDiscoveryRows().length === 0, 'and nothing is drawn');

  // --- the change event only refetches connections that are actually watched ---
  reset();
  const before = fake.calls.filter((c) => c.method === 'GetDiscoveryTree').length;
  onDiscoveryTreeChanged('c1');
  assert(
    fake.calls.filter((c) => c.method === 'GetDiscoveryTree').length === before,
    'a change for a collapsed connection is ignored, not fetched'
  );
  onDiscoveryTreeChanged('');
  assert(
    fake.calls.filter((c) => c.method === 'GetDiscoveryTree').length === before,
    'an event with no connectionId is ignored'
  );

  // --- forgetDiscoveryTree is safe on a connection that has nothing ---
  reset();
  forgetDiscoveryTree('nope');
  assert(get(discoveryExpanded).size === 0, 'forgetting an unknown connection is a no-op');

  // --- icons are keyed by plugin: two plugins may both ship "volumes" ---
  reset();
  fake.program('ListPlugins', [
    { id: 'p1', discoveryIcons: { volumes: 'data:image/svg+xml;base64,AAA' } },
    { id: 'p2', discoveryIcons: { volumes: 'data:image/svg+xml;base64,BBB' } },
    { id: 'p3' },
  ]);
  await refreshDiscoveryIcons();
  const icons = get(discoveryIcons);
  assert(icons.size === 2, `both icons are kept, got ${icons.size}`);
  assert(
    icons.get(discoveryIconKey('p1', 'volumes')) === 'data:image/svg+xml;base64,AAA' &&
      icons.get(discoveryIconKey('p2', 'volumes')) === 'data:image/svg+xml;base64,BBB',
    'an iconId is only meaningful together with its plugin'
  );

  // --- and the tree actually calls it ---
  //
  // Everything above proves the store function does the right thing; none of it
  // proves anyone invokes it, and the bug this replaces was exactly that —
  // forgetDiscoveryTree existed, was exported, and was called from nowhere in
  // src/. RemoteTree.svelte cannot be rendered here (no DOM, no Svelte runtime),
  // so the link is asserted on the source, the same technique architecture.test.ts
  // uses. Deleting the wiring must fail this.
  {
    const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), '..', 'lib', 'RemoteTree.svelte'), 'utf8');
    const code = src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
    assert(
      /\$:\s*forgetUnavailableDiscovery\(\s*discoveryAvailableIds\s*\)/.test(code),
      'RemoteTree.svelte must reactively call forgetUnavailableDiscovery(discoveryAvailableIds); ' +
        'without it an expanded subtree outlives its session with no control left to collapse it.'
    );
    assert(
      /discoveryAvailableIds\s*=\s*new Set\(\s*\$sessions\.filter\(\(s\) => s\.state === 'ready'\)/.test(code),
      "discoveryAvailableIds must be built from `ready` sessions only — ADR-014's leading session"
    );

    // Position, not just presence. Svelte orders reactive blocks topologically by
    // their tracked dependencies, and there is NO dependency between these two —
    // the cleanup writes to a store the builder reads, which the compiler cannot
    // see. So the order is the source order, and moving the cleanup below the
    // builder would compile, compute `tree` from a stale set, and reproduce the
    // original bug without any test noticing.
    const cleanupAt = code.indexOf('forgetUnavailableDiscovery(discoveryAvailableIds)');
    const buildAt = code.indexOf('tree = buildTree(');
    assert(cleanupAt >= 0 && buildAt >= 0, 'both reactive blocks must exist');
    assert(
      cleanupAt < buildAt,
      'forgetUnavailableDiscovery must appear BEFORE tree = buildTree(...): there is no tracked ' +
        'dependency between them, so their source order is what makes the cleanup land in the same pass.'
    );

    // Focus must be addressed by the connection-scoped TreeNode id, never by the
    // bare discoveryKey — one plugin publishing the same node on two hosts yields
    // two rows with the same key, and querySelector would take whichever came
    // first in the document.
    assert(
      /data-discovery-id="\$\{escaped\}"/.test(code) && /discoveryNodeId\(\s*row\.connectionId/.test(code),
      'the focus lookup must select on data-discovery-id built from discoveryNodeId(connectionId, ...)'
    );
    assert(
      !/data-discovery-key/.test(code),
      'the connection-less data-discovery-key addressing must not come back'
    );
  }

  // ...and the row must publish that same connection-scoped id.
  {
    const rowSrc = readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), '..', 'lib', 'remoteTree', 'RemoteTreeNode.svelte'),
      'utf8'
    );
    assert(
      /data-discovery-id=\{node\.discovery \? node\.id : null\}/.test(rowSrc),
      'RemoteTreeNode must emit data-discovery-id from node.id (connection-scoped), not from node.discovery.key'
    );
  }

  console.log('discoveryState.test.ts: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
