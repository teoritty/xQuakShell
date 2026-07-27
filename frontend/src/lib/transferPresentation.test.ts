import {
  kindLabel,
  showsRate,
  isScanning,
  refreshesRemotePane,
  refreshesLocalPane,
  remoteRefreshDirs,
} from './transferPresentation';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

// kindLabel: every OperationKind maps to a distinct human label. localcopy
// gets its own label ('Copy') rather than silently reusing 'Upload' — that
// was the whole point of removing the backend's upload rewrite.
{
  assert(kindLabel('upload') === 'Upload', 'upload label');
  assert(kindLabel('download') === 'Download', 'download label');
  assert(kindLabel('localcopy') === 'Copy', 'localcopy label');
  assert(kindLabel('delete') === 'Delete', 'delete label');
  assert(kindLabel('chmod') === 'chmod', 'chmod label');
  assert(kindLabel('chown') === 'chown', 'chown label');
}

// kindLabel falls back gracefully for a value outside the known union (e.g.
// stale client vs. newer backend).
{
  assert(kindLabel('bogus' as any) === 'Operation', 'unknown kind falls back to Operation');
}

// showsRate: byte-moving kinds show a rate while active; localcopy included.
{
  assert(showsRate('upload', 'active') === true, 'upload active shows rate');
  assert(showsRate('download', 'active') === true, 'download active shows rate');
  assert(showsRate('localcopy', 'active') === true, 'localcopy active shows rate');
  assert(showsRate('delete', 'active') === false, 'delete never shows rate');
  assert(showsRate('chmod', 'active') === false, 'chmod never shows rate');
  assert(showsRate('chown', 'active') === false, 'chown never shows rate');
  assert(showsRate('upload', 'completed') === false, 'non-active upload shows no rate');
  assert(showsRate('localcopy', 'pending') === false, 'non-active localcopy shows no rate');
}

// isScanning: active + unknown total, independent of kind. This is the
// property that replaced the old kind-enumeration predicate.
{
  assert(isScanning('active', 0) === true, 'active + total 0 is scanning');
  assert(isScanning('active', -1) === true, 'active + negative total is scanning');
  assert(isScanning('active', 100) === false, 'active + known total is not scanning');
  assert(isScanning('completed', 0) === false, 'completed is never scanning');
  assert(isScanning('pending', 0) === false, 'pending is never scanning');
}

// refreshesRemotePane: only operations that touched *this* session's remote
// tree. The session check also keeps a local copy (no session) out of every
// remote pane.
{
  assert(refreshesRemotePane('upload', 's1', 's1') === true, 'upload refreshes its own remote pane');
  assert(refreshesRemotePane('upload', 's2', 's1') === false, 'another session\'s upload is ignored');
  assert(refreshesRemotePane('delete', 's1', 's1') === true, 'delete refreshes the remote pane');
  assert(refreshesRemotePane('chmod', 's1', 's1') === true, 'chmod refreshes the remote pane');
  assert(refreshesRemotePane('chown', 's1', 's1') === true, 'chown refreshes the remote pane');
  assert(refreshesRemotePane('download', 's1', 's1') === false, 'download changes nothing remote');
  assert(refreshesRemotePane('localcopy', undefined, 's1') === false, 'a local copy has no session');
}

// refreshesLocalPane: downloads and Explorer drops write to the host FS.
// A local copy is recognised by its own kind — it is no longer rewritten to
// 'upload' by the backend, so no session-less-upload special case is needed.
{
  assert(refreshesLocalPane('download') === true, 'download refreshes the local pane');
  assert(refreshesLocalPane('localcopy') === true, 'localcopy refreshes the local pane');
  assert(refreshesLocalPane('upload') === false, 'upload writes nothing locally');
  assert(refreshesLocalPane('delete') === false, 'remote delete writes nothing locally');
}

// remoteRefreshDirs: refreshDir is authoritative and used verbatim. Only a
// recursive chmod/chown also needs the parent, because the operated
// directory's own mode is rendered one level up.
{
  const eq = (a: string[], b: string[]) => a.length === b.length && a.every((v, i) => v === b[i]);
  assert(eq(remoteRefreshDirs('upload', '/var/www'), ['/var/www']), 'upload reloads the destination');
  assert(eq(remoteRefreshDirs('delete', '/srv'), ['/srv']), 'delete reloads the parent the backend chose');
  assert(eq(remoteRefreshDirs('chmod', '/srv/logs'), ['/srv/logs', '/srv']), 'chmod reloads the dir and its parent');
  assert(eq(remoteRefreshDirs('chown', '/srv/logs'), ['/srv/logs', '/srv']), 'chown reloads the dir and its parent');
  assert(eq(remoteRefreshDirs('chmod', '/top'), ['/top', '/']), 'a top-level chmod bottoms out at the root');
}

console.log('transferPresentation.test.ts: all passed');
