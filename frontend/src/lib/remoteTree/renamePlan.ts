// Deciding a rename is kept apart from performing one, because the editor has to close either way.
//
// Both inline editors used to clear their `editing*Id` only after the save had come back. That made
// the row's edit mode a hostage of a backend round-trip: anything that failed or never resolved
// between the blur and that line left the input open with no way out but clicking into it and out
// again. The editor belongs to the interaction, and the interaction is over the moment the input is
// dismissed, so the caller clears the id first and hands the plan to the save.
export interface Renameable {
  id: string;
  name: string;
}

export interface RenamePlan<T extends Renameable> {
  target: T;
  name: string;
}

/**
 * The rename to carry out, or null when there is nothing to do: no editor open, a name trimmed away
 * to nothing (the row keeps the one it had), a row that has since vanished, or a name that did not
 * actually change — a rename that renames nothing must not rewrite the record, because saving a
 * whole connection is never free of side effects.
 */
export function planRename<T extends Renameable>(
  editingId: string | null,
  draftName: string,
  items: T[],
): RenamePlan<T> | null {
  const name = draftName.trim();
  if (!editingId || !name) return null;
  const target = items.find((item) => item.id === editingId);
  if (!target || target.name === name) return null;
  return { target, name };
}
