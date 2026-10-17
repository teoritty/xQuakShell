// When, and how, the connection editor must re-read the record it is showing.
//
// The editor holds a draft, and the draft is a copy. Deciding to refresh it by comparing connection
// ids made "the same connection" mean "the same content": renaming a connection from the tree
// changes the record and not its id, so the panel went on showing the old name — and its next
// autosave submitted that stale name and undid the rename.
//
// The name was once the only field followed, on the reasoning that every other field had the panel
// as its only writer. Sync ended that: another device changes a host or a port while this panel is
// open, the panel kept showing the old value, and an edit of any other field saved the whole stale
// draft — reverting the other device's change on every device, with no conflict, because to the
// merge it was simply a newer edit. So the whole record is followed now.
//
// THE COMPARISON IS RECORD-AGAINST-RECORD, and that is not a detail. Asking whether the draft
// differs from the record cannot say which of the two moved: a keystroke moves the draft, and
// reading that as "the record changed" wrote the record back over every character typed. Comparing
// the record against the one the panel last adopted asks the question that has an answer - only the
// record moving is a change from elsewhere, and typing never moves the record.
//
// Following does not throw edits away. rebaseDraft (draftRebase.ts) takes from the new record exactly the fields the
// person has not touched since the panel adopted the old one - a three-way merge per field, with
// the person's unsaved edit winning where both changed - so an open, half-edited panel picks up the
// other device's host while keeping the name being typed, and the next autosave carries both.
import type { Connection } from '../../stores/appState';
import type { ConnectionProtocol } from '../../actions/protocolActions';
import type { ConnectionDetailsDraft } from './types';
import { createDraftFromConnection, resolveDefaultPort } from './connectionDraft';
import { applyProtocolFieldDefaults } from './connectionFormMode';
import { rebaseDraft } from '../draftRebase';

export type DraftSyncAction = 'rebuild' | 'catalog' | 'follow-record' | 'none';

export interface DraftSyncInputs {
  /** Id of the connection the panel is showing; empty when the panel is closed. */
  connId: string;
  /** Id the current draft was built from. */
  draftId: string;
  /** The record as it stands now, as recordKey renders it. */
  recordKey: string;
  /** The record the panel last adopted, rendered the same way. */
  boundRecordKey: string;
  /** Identity of the protocol catalog, and the one the draft's fields were bound against. */
  catalogKey: string;
  boundCatalogKey: string;
}

/** A record's identity for "did it move": its whole content, so any field changed elsewhere counts. */
export function recordKey(record: Connection | null | undefined): string {
  return JSON.stringify(record ?? null);
}

export function nextDraftSync(input: DraftSyncInputs): DraftSyncAction {
  if (!input.connId) return 'none';
  if (input.connId !== input.draftId) return 'rebuild';
  if (input.catalogKey !== input.boundCatalogKey) return 'catalog';
  if (input.recordKey !== input.boundRecordKey) return 'follow-record';
  return 'none';
}

/** rebaseDraft for the connection editor: both records become drafts the way the panel builds one. */
export function followRecord(
  draft: ConnectionDetailsDraft,
  bound: Connection | null,
  latest: Connection | null,
  protocols: ConnectionProtocol[],
): ConnectionDetailsDraft {
  const asDraft = (c: Connection | null) => {
    const d = createDraftFromConnection(c, resolveDefaultPort(c?.protocol || 'ssh', protocols, c?.port));
    applyProtocolFieldDefaults(d, d.protocol, protocols);
    return d;
  };
  return rebaseDraft(draft, asDraft(bound), asDraft(latest));
}
