import { followRecord, nextDraftSync, recordKey, type DraftSyncInputs } from './draftSync';
import { rebaseDraft } from '../draftRebase';
import { createDraftFromConnection } from './connectionDraft';
import type { Connection } from '../../stores/appState';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const desk: Connection = { id: 'c1', name: 'Desk', host: 'old.example', port: 22, protocol: 'ssh' } as Connection;

// An open panel, showing the record it last adopted, with nothing pending.
function settled(over: Partial<DraftSyncInputs> = {}): DraftSyncInputs {
  return {
    connId: 'c1',
    draftId: 'c1',
    recordKey: recordKey(desk),
    boundRecordKey: recordKey(desk),
    catalogKey: 'cat-1',
    boundCatalogKey: 'cat-1',
    ...over,
  };
}

assert(nextDraftSync(settled()) === 'none', 'a settled panel must not re-read anything');
assert(nextDraftSync(settled({ connId: '' })) === 'none', 'a closed panel has nothing to sync');

// A rename from the tree and a host changed on another device both move the record, never its id.
assert(
  nextDraftSync(settled({ recordKey: recordKey({ ...desk, name: 'Desk (office)' }) })) === 'follow-record',
  'a name changed elsewhere must reach the panel',
);
assert(
  nextDraftSync(settled({ recordKey: recordKey({ ...desk, host: 'new.example' }) })) === 'follow-record',
  'a host changed on another device must reach the panel, not only a rename',
);

// A different connection is a full rebuild, and it wins over every other case.
assert(nextDraftSync(settled({ draftId: 'c2' })) === 'rebuild', 'a new connection rebuilds the draft');
assert(
  nextDraftSync(settled({ draftId: 'c2', recordKey: recordKey({ ...desk, name: 'Other' }) })) === 'rebuild',
  'switching connections rebuilds whatever else moved',
);

// A catalog change re-binds the protocol fields and is decided first; the record is followed on the
// next pass, because re-binding records the new catalog key and not the new record.
assert(nextDraftSync(settled({ catalogKey: 'cat-2' })) === 'catalog', 'a new catalog re-binds fields');
assert(
  nextDraftSync(settled({ catalogKey: 'cat-2', recordKey: recordKey({ ...desk, name: 'Renamed' }) })) === 'catalog',
  'the catalog is decided before the record',
);

// rebaseDraft is a per-field three-way merge.
{
  const base = { name: 'a', host: 'h1', port: 22 };
  const draft = { name: 'typed', host: 'h1', port: 22 };
  const latest = { name: 'a', host: 'h2', port: 2222 };
  const out = rebaseDraft(draft, base, latest);
  assert(out.name === 'typed', 'the person\'s unsaved edit survives the record moving');
  assert(out.host === 'h2' && out.port === 2222, 'every field the person did not touch follows the record');
  const both = rebaseDraft({ ...draft, host: 'mine' }, base, latest);
  assert(both.host === 'mine', 'where both changed the same field, the edit on screen wins and autosaves');
}

// The data loss this exists to stop, end to end: another device changed the host while the panel
// was open; the person then renamed the connection. The draft that autosaves must carry the other
// device's host, not the one the panel opened with.
{
  const opened = createDraftFromConnection(desk, 22);
  const renamed = { ...opened, name: 'Desk renamed' };
  const remote: Connection = { ...desk, host: 'moved.example' };
  const followed = followRecord(renamed, desk, remote, []);
  assert(followed.host === 'moved.example', `host = ${followed.host}; saving the stale host would revert the other device everywhere`);
  assert(followed.name === 'Desk renamed', 'the rename being typed is kept');
  const untouched = followRecord(opened, desk, remote, []);
  assert(untouched.host === 'moved.example' && untouched.name === 'Desk', 'an untouched panel shows the record as it now is');
}

console.log('draftSync tests passed');
