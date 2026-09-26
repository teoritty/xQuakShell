import assert from 'node:assert/strict';
import { listForPane, settleListing, isDirectoryNotFound, type PaneTreeState } from './missingDir';
import { remoteParent, parentDirectory } from './paths';

// A backend over a fixed set of directories. A missing one fails the way the real RPC does: an
// Error carrying domain.ErrDirectoryNotFound's message, nothing else.
function backend(existing: string[]) {
  const calls: string[] = [];
  const list = async (path: string): Promise<string[]> => {
    calls.push(path);
    if (path === '/denied') throw new Error('permission denied');
    if (!existing.includes(path)) throw new Error('directory not found');
    return [`${path}/child`];
  };
  return { list, calls };
}

function freshState(): PaneTreeState<string> {
  return { tree: new Map(), rawTree: new Map(), expanded: new Set() };
}

const identity = (nodes: string[]) => nodes;

async function run() {
  // The bug: the directory on screen was deleted. The pane lands on the nearest parent that is
  // still there, skipping any that went with it, and does not raise.
  {
    const { list, calls } = backend(['/', '/home']);
    const listing = await listForPane('/home/u/proj', '/home/u/proj', list, remoteParent);
    assert.equal(listing.path, '/home', 'climbs past every deleted level to the first one that exists');
    assert.deepEqual(listing.nodes, ['/home/child'], 'and lists it');
    assert.deepEqual(calls, ['/home/u/proj', '/home/u', '/home'], 'one call per level, nearest first');

    const state = freshState();
    state.tree.set('/home/u/proj', ['stale']);
    state.expanded.add('/home/u/proj');
    const current = settleListing(state, listing, '/home/u/proj', identity);
    assert.equal(current, '/home', 'the pane now shows the parent');
    assert.ok(!state.tree.has('/home/u/proj'), 'the vanished directory is gone from the tree');
    assert.ok(!state.expanded.has('/home/u/proj'), 'and from the expanded set');
    assert.ok(state.expanded.has('/home'), 'the new current directory is expanded');
    assert.deepEqual(state.tree.get('/home'), ['/home/child']);
  }

  // A directory that was only expanded, not shown, has gone: it is dropped, and the pane stays put.
  {
    const { list, calls } = backend(['/srv']);
    const listing = await listForPane('/srv/old', '/srv', list, remoteParent);
    assert.equal(listing.nodes, null, 'a vanished non-current directory is reported as gone');
    assert.deepEqual(calls, ['/srv/old'], 'without climbing: the pane is not showing it');

    const state = freshState();
    state.tree.set('/srv/old', ['stale']);
    state.expanded.add('/srv/old');
    assert.equal(settleListing(state, listing, '/srv', identity), '/srv', 'the current directory is unchanged');
    assert.ok(!state.tree.has('/srv/old') && !state.expanded.has('/srv/old'), 'the vanished branch is dropped');
  }

  // Nothing is lost on the common path.
  {
    const { list } = backend(['/srv']);
    const listing = await listForPane('/srv', '/srv', list, remoteParent);
    const state = freshState();
    assert.equal(settleListing(state, listing, '/srv', identity), '/srv');
    assert.deepEqual(state.tree.get('/srv'), ['/srv/child'], 'an existing directory lists as before');
  }

  // Only a missing directory is absorbed. Anything else still reaches the pane as an error.
  {
    const { list } = backend(['/']);
    await assert.rejects(listForPane('/denied', '/denied', list, remoteParent), /permission denied/,
      'a permission error is not mistaken for a deleted directory');
  }

  // With nowhere left to climb the failure is real and is rethrown rather than looping.
  {
    const { list } = backend([]);
    await assert.rejects(listForPane('/a', '/a', list, remoteParent), /directory not found/,
      'a missing root is rethrown');
  }

  // Local Windows paths climb by the same rule, stopping at the drive root.
  {
    const { list } = backend(['C:\\Users']);
    const listing = await listForPane('C:\\Users\\u\\gone', 'C:\\Users\\u\\gone', list, parentDirectory);
    assert.equal(listing.path, 'C:\\Users', 'a local pane climbs through Windows parents too');
  }

  assert.ok(isDirectoryNotFound(new Error('directory not found')));
  assert.ok(isDirectoryNotFound('List remote path: directory not found'), 'a string message is recognised');
  assert.ok(!isDirectoryNotFound(new Error('connection lost')));
  assert.ok(!isDirectoryNotFound(undefined));

  console.log('OK fileTree/missingDir');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
