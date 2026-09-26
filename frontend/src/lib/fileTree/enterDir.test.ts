import assert from 'node:assert/strict';
import { createDirectoryEntry } from './enterDir';
import type { PaneTreeState } from './missingDir';

function freshState(): PaneTreeState<string> {
  return { tree: new Map(), rawTree: new Map(), expanded: new Set() };
}

// A listing the test releases by hand, so two requests can finish in either order.
function deferred() {
  let resolve!: (nodes: string[]) => void;
  let reject!: (e: Error) => void;
  const promise = new Promise<string[]>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

const reversed = (nodes: string[]) => [...nodes].reverse();

async function run() {
  // A readable directory is listed, sorted into the tree, kept raw, and marked expanded.
  {
    const state = freshState();
    const enter = createDirectoryEntry(async (p) => [`${p}/a`, `${p}/b`], reversed);
    assert.equal(await enter(state, '/home/u'), 'entered');
    assert.deepEqual(state.rawTree.get('/home/u'), ['/home/u/a', '/home/u/b'], 'the raw listing is kept for re-sorting');
    assert.deepEqual(state.tree.get('/home/u'), ['/home/u/b', '/home/u/a'], 'the tree holds the sorted listing');
    assert.ok(state.expanded.has('/home/u'), 'the entered directory is expanded');
  }

  // The bug: a directory the user cannot read. The failure reaches the pane and nothing about the
  // pane's state changes, so it stays in the directory it was in.
  {
    const state = freshState();
    state.tree.set('/home/u', ['/home/u/x']);
    state.rawTree.set('/home/u', ['/home/u/x']);
    state.expanded.add('/home/u');
    const enter = createDirectoryEntry<string>(async () => { throw new Error('sftp list /root: permission denied'); }, reversed);
    await assert.rejects(enter(state, '/root'), /permission denied/, 'an unreadable directory is reported, not entered');
    assert.ok(!state.tree.has('/root') && !state.rawTree.has('/root'), 'no empty listing is stored for it');
    assert.ok(!state.expanded.has('/root'), 'it is not marked expanded');
    assert.deepEqual([...state.tree.keys()], ['/home/u'], 'the directory the pane was in is untouched');
  }

  // Only the latest request counts: a slow directory opened first must not win over a fast one
  // opened after it.
  {
    const state = freshState();
    const pending = new Map([['/slow', deferred()], ['/fast', deferred()]]);
    const enter = createDirectoryEntry((p) => pending.get(p)!.promise, reversed);
    const slow = enter(state, '/slow');
    const fast = enter(state, '/fast');
    pending.get('/fast')!.resolve(['/fast/f']);
    assert.equal(await fast, 'entered');
    pending.get('/slow')!.resolve(['/slow/s']);
    assert.equal(await slow, 'superseded', 'an overtaken request reports it was superseded');
    assert.ok(!state.tree.has('/slow') && !state.expanded.has('/slow'), 'an overtaken listing is not stored');
    assert.ok(state.tree.has('/fast'), 'the latest listing is');
  }

  // An overtaken request that fails does not raise: its error belongs to a directory the pane is
  // no longer heading for.
  {
    const state = freshState();
    const pending = new Map([['/root', deferred()], ['/tmp', deferred()]]);
    const enter = createDirectoryEntry((p) => pending.get(p)!.promise, reversed);
    const denied = enter(state, '/root');
    const next = enter(state, '/tmp');
    pending.get('/root')!.reject(new Error('permission denied'));
    assert.equal(await denied, 'superseded', 'a superseded failure resolves quietly');
    pending.get('/tmp')!.resolve([]);
    assert.equal(await next, 'entered', 'an empty readable directory is still entered');
    assert.deepEqual(state.tree.get('/tmp'), [], 'and listed as empty');
  }

  console.log('OK fileTree/enterDir');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
