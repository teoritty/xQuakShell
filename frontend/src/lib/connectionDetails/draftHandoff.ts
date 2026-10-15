// What happens to an edit when the editor moves to another connection before the edit was saved.
//
// The editor saves 600ms after the last keystroke, and a password only after importing it into the
// vault. Opening another connection inside that window used to invalidate the pending save: the
// name typed a moment ago was gone, and so was the password - the most common case, because typing
// a password and then clicking the next server in the tree takes well under a second. The edit
// belongs to the connection it was made on, so it is saved there, whatever the editor shows now.
import { get } from 'svelte/store';
import { connections, type Connection } from '../../stores/appState';
import { setHopPassword, setUserPassword } from './authDraftMutations';
import { buildConnectionSavePayload } from './savePayload';
import type { ConnectionDetailsDraft } from './types';

/**
 * Saves draft as an edit of record now, outside the editor's debounce. A connection that is gone -
 * deleted, or moved to another vault under a new id, which also moves the editor - is not saved: a
 * save under its old id would bring it back as a second copy.
 */
export async function saveDraftNow(
  draft: ConnectionDetailsDraft,
  record: Connection | null,
  save: (c: Partial<Connection>) => Promise<unknown>,
): Promise<void> {
  if (!draft.editingId || !draft.name.trim()) return;
  if (!get(connections).some((c) => c.id === draft.editingId)) return;
  await save(buildConnectionSavePayload(draft, { folderId: record?.folderId || '', order: record?.order ?? 0 }));
}

/** draft with passwordId set on the user or hop named, or null when draft has neither any more. */
export function withPassword(
  draft: ConnectionDetailsDraft,
  target: { userId?: string; hopId?: string },
  passwordId: string,
): ConnectionDetailsDraft | null {
  if (target.userId && draft.users.some((u) => u.id === target.userId)) {
    return { ...draft, users: setUserPassword(draft.users, target.userId, passwordId) };
  }
  if (target.hopId && draft.jumpHops.some((h) => h.id === target.hopId)) {
    return { ...draft, jumpHops: setHopPassword(draft.jumpHops, target.hopId, passwordId) };
  }
  return null;
}
