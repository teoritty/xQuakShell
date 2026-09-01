import { nextDraftSync, type DraftSyncInputs } from './draftSync';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

// An open panel, showing the record it was built from, with nothing pending.
function settled(over: Partial<DraftSyncInputs> = {}): DraftSyncInputs {
  return {
    connId: 'c1',
    draftId: 'c1',
    draftName: 'Desk',
    recordName: 'Desk',
    dirty: false,
    catalogKey: 'cat-1',
    boundCatalogKey: 'cat-1',
    ...over,
  };
}

assert(nextDraftSync(settled()) === 'none', 'a settled panel must not re-read anything');
assert(nextDraftSync(settled({ connId: '' })) === 'none', 'a closed panel has nothing to sync');

// The reported defect: renaming from the tree changes the record, never its id.
assert(
  nextDraftSync(settled({ recordName: 'Desk (office)' })) === 'follow-name',
  'a name changed elsewhere must reach the panel',
);

// A save stores the trimmed name, so a draft that differs only by that trim is already in sync.
// Treating it as an external rename would copy the record back mid-edit and jump the caret.
assert(
  nextDraftSync(settled({ draftName: 'Desk  ' })) === 'none',
  'the payload trim is not an external rename',
);

// The user's own unsaved edit outranks it — that edit is what the record is about to become.
assert(
  nextDraftSync(settled({ recordName: 'Desk (office)', dirty: true })) === 'none',
  'a dirty draft must never be overwritten from the record',
);

// A different connection is a full rebuild, and it wins over both other cases.
assert(nextDraftSync(settled({ draftId: 'c2' })) === 'rebuild', 'a new connection rebuilds the draft');
assert(
  nextDraftSync(settled({ draftId: 'c2', dirty: true, recordName: 'Other' })) === 'rebuild',
  'switching connections rebuilds even from a dirty draft',
);

// A catalog change re-binds the protocol fields and is decided first. The name is not lost by that:
// re-binding records the new catalog key, so the next pass sees only the stale name and follows it.
assert(nextDraftSync(settled({ catalogKey: 'cat-2' })) === 'catalog', 'a new catalog re-binds fields');
assert(
  nextDraftSync(settled({ catalogKey: 'cat-2', recordName: 'Renamed' })) === 'catalog',
  'the catalog is decided before the name',
);

console.log('draftSync tests passed');
