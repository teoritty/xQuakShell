import { nextDraftSync, type DraftSyncInputs } from './draftSync';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

// An open panel, showing the record it last adopted, with nothing pending.
function settled(over: Partial<DraftSyncInputs> = {}): DraftSyncInputs {
  return {
    connId: 'c1',
    draftId: 'c1',
    recordName: 'Desk',
    boundRecordName: 'Desk',
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

// Typing moves the draft and leaves the record alone, so it can never look like a rename made
// elsewhere. This is the assertion that matters: `dirty` lags a keystroke by one reactive pass, so
// asking "does the draft differ from the record" answered "yes, and nothing is unsaved" mid-word and
// wrote the record back over every character typed.
assert(
  nextDraftSync(settled({ recordName: 'Desk', boundRecordName: 'Desk', dirty: false })) === 'none',
  'a draft that moved while the record stood still is not a rename',
);

// The user's own unsaved edit outranks a rename from elsewhere — the autosave settles the two.
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
// re-binding records the new catalog key and not the new record name, so the next pass follows it.
assert(nextDraftSync(settled({ catalogKey: 'cat-2' })) === 'catalog', 'a new catalog re-binds fields');
assert(
  nextDraftSync(settled({ catalogKey: 'cat-2', recordName: 'Renamed' })) === 'catalog',
  'the catalog is decided before the name',
);

console.log('draftSync tests passed');
