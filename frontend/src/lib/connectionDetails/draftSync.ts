// When the connection editor must re-read the record it is showing.
//
// The editor holds a draft, and the draft is a copy. Deciding to refresh it by comparing connection
// ids made "the same connection" mean "the same content": renaming a connection from the tree
// changes the record and not its id, so the panel went on showing the old name — for the rest of
// the session, because closing and reopening the panel does not change the id either. Worse, the
// panel's next autosave submitted that stale name and undid the rename.
//
// `name` is the only persisted field the draft caches that anything outside the panel can change.
// Moving and reordering a connection touch `folderId` and `order`, which the draft does not hold
// (they are read from the record at save time), and every other field has the panel as its only
// writer. If that ever stops being true, this is the decision that has to grow a case for it.
//
// THE COMPARISON IS RECORD-AGAINST-RECORD, and that is not a detail. Asking whether the draft
// differs from the record cannot say which of the two moved, and the answer was taken from `dirty`
// - which lags by one pass. A keystroke reaches the reactive statement as a changed draft while
// `dirty` is still false, so every character typed into the name field was read as "the record was
// renamed" and written straight back. The field could not be typed in at all: a selection replaced
// by a keystroke came back whole, and the panel looked frozen. Comparing the record against the one
// the panel last adopted asks the question that has an answer - only the record moving is a rename
// from elsewhere, and typing never moves the record.
//
// `dirty` still guards the write, but as a second condition rather than the deciding one: an
// unsaved edit outranks a rename made elsewhere, and the autosave that follows resolves the two.
export type DraftSyncAction = 'rebuild' | 'catalog' | 'follow-name' | 'none';

export interface DraftSyncInputs {
  /** Id of the connection the panel is showing; empty when the panel is closed. */
  connId: string;
  /** Id the current draft was built from. */
  draftId: string;
  /** Name on the record as it stands now. */
  recordName: string;
  /** Name the record carried when the panel last adopted it. */
  boundRecordName: string;
  dirty: boolean;
  /** Identity of the protocol catalog, and the one the draft's fields were bound against. */
  catalogKey: string;
  boundCatalogKey: string;
}

export function nextDraftSync(input: DraftSyncInputs): DraftSyncAction {
  if (!input.connId) return 'none';
  if (input.connId !== input.draftId) return 'rebuild';
  if (input.catalogKey !== input.boundCatalogKey) return 'catalog';
  if (!input.dirty && input.recordName !== input.boundRecordName) return 'follow-name';
  return 'none';
}
