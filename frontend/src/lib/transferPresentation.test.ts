import { kindLabel, showsRate, isScanning } from './transferPresentation';

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

console.log('transferPresentation.test.ts: all passed');
