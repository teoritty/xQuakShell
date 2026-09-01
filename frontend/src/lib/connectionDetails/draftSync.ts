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
// A dirty draft is never overwritten: the user's unsaved edit outranks a change from elsewhere, and
// the autosave that follows resolves the two.
export type DraftSyncAction = 'rebuild' | 'catalog' | 'follow-name' | 'none';

export interface DraftSyncInputs {
  /** Id of the connection the panel is showing; empty when the panel is closed. */
  connId: string;
  /** Id the current draft was built from. */
  draftId: string;
  draftName: string;
  /** Name on the record as it stands now. */
  recordName: string;
  dirty: boolean;
  /** Identity of the protocol catalog, and the one the draft's fields were bound against. */
  catalogKey: string;
  boundCatalogKey: string;
}

export function nextDraftSync(input: DraftSyncInputs): DraftSyncAction {
  if (!input.connId) return 'none';
  if (input.connId !== input.draftId) return 'rebuild';
  if (input.catalogKey !== input.boundCatalogKey) return 'catalog';
  if (!input.dirty && input.recordName !== input.draftName) return 'follow-name';
  return 'none';
}
