// Wording and severity of the file panes' delete confirmation.
//
// This was a three-deep nested ternary duplicated in both panes' markup, which
// is the worst place for it: a destructive action's copy, and whether the user
// has to tick "I understand" at all, decided by an expression no test can
// reach. Deleting many items, or one directory that is not empty, is the case
// that must not quietly lose its checkbox.

export interface DeleteTargets {
  /** Paths from a multi-row selection; empty for a single-target delete. */
  pathsToDelete: readonly string[];
  /** Display name of a single target. */
  name: string;
  /** Entries inside a single directory target. */
  childCount: number;
}

export interface DeletePrompt {
  title: string;
  message: string;
  critical: boolean;
  requireCheckbox: boolean;
}

export function describeDelete(targets: DeleteTargets): DeletePrompt {
  const bulk = targets.pathsToDelete.length > 1 || targets.childCount > 0;
  return {
    title: bulk ? 'Delete items?' : 'Delete?',
    message: deleteMessage(targets),
    critical: bulk,
    requireCheckbox: bulk,
  };
}

function deleteMessage(targets: DeleteTargets): string {
  const count = targets.pathsToDelete.length;
  if (count > 0) {
    return `You are deleting ${count} item(s). This action cannot be undone.`;
  }
  if (targets.childCount > 0) {
    return `You are deleting "${targets.name}" and ${targets.childCount} item(s) inside. This action cannot be undone.`;
  }
  return `Delete "${targets.name}"?`;
}
