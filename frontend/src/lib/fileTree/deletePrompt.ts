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

/** Resolves a message key. The panes pass `$t`; tests pass whatever they need. */
export type PromptLabels = (key: string, vars?: Record<string, string | number>) => string;

export function describeDelete(targets: DeleteTargets, label: PromptLabels): DeletePrompt {
  const bulk = targets.pathsToDelete.length > 1 || targets.childCount > 0;
  return {
    title: label(bulk ? 'files.delete.many.title' : 'files.delete.one.title'),
    message: deleteMessage(targets, label),
    critical: bulk,
    requireCheckbox: bulk,
  };
}

// The counts go through the lookup as `count`, which also picks the plural form: this dialog
// exists to make the user read a number, and "1 items" is what makes a reader skim past it.
function deleteMessage(targets: DeleteTargets, label: PromptLabels): string {
  const count = targets.pathsToDelete.length;
  if (count > 0) {
    return label('security.files.delete.many', { count });
  }
  if (targets.childCount > 0) {
    return label('security.files.delete.directory', { name: targets.name, count: targets.childCount });
  }
  return label('files.delete.one', { name: targets.name });
}
