import type { Connection, Folder } from '../../stores/appState';
import { countConnectionsInFolder } from './buildTree';
import type { DeleteTargets } from './selection';

export interface DeletePrompt {
  title: string;
  message: string;
  /** Drives both the red styling and the "I understand" checkbox. */
  critical: boolean;
  checkboxLabel: string;
}

/** Resolves a message key. The tree passes `$t`; tests pass whatever they need. */
export type PromptLabels = (key: string, vars?: Record<string, string | number>) => string;

/**
 * Wording for the delete confirmation, kept out of the component because it is
 * the only thing standing between the user and an irreversible cascade: a
 * folder delete takes its whole subtree with it, so the dialog has to say how
 * many connections that actually is. Pure and separate so those counts can be
 * tested rather than eyeballed in the UI.
 *
 * The counts go through the lookup as `count`, which also selects the plural form. That matters
 * here more than anywhere else in the interface: this dialog exists to make the user read a
 * number, and "1 connections" is the kind of wrongness that makes a reader skim past it.
 */
export function describeDeleteTargets(
  targets: DeleteTargets,
  folders: Folder[],
  connections: Connection[],
  label: PromptLabels,
): DeletePrompt {
  const folderCount = targets.folderIds.length;
  const connCount = targets.connectionIds.length;
  const nested = targets.folderIds.reduce(
    (sum, id) => sum + countConnectionsInFolder(id, folders, connections),
    0
  );
  const critical = folderCount + connCount > 1 || nested > 0;
  const checkboxLabel = label(
    folderCount > 0
      ? 'security.tree.delete.confirmFolders'
      : 'security.tree.delete.confirmConnections',
  );

  if (folderCount === 1 && connCount === 0) {
    const name = folders.find((f) => f.id === targets.folderIds[0])?.name ?? '';
    if (nested === 0) {
      return {
        title: label('tree.delete.folder.title'),
        message: label('tree.delete.one', { name }),
        critical,
        checkboxLabel,
      };
    }
    return {
      title: label('security.tree.delete.folderNotEmpty.title'),
      message: label('security.tree.delete.folderNotEmpty', { name, count: nested }),
      critical,
      checkboxLabel,
    };
  }

  if (folderCount === 0 && connCount === 1) {
    const name = connections.find((c) => c.id === targets.connectionIds[0])?.name ?? '';
    return {
      title: label('tree.delete.connection.title'),
      message: label('tree.delete.one', { name }),
      critical,
      checkboxLabel,
    };
  }

  if (folderCount === 0) {
    return {
      title: label('tree.delete.connections.title'),
      message: label('security.tree.delete.connections', { count: connCount }),
      critical,
      checkboxLabel,
    };
  }

  const subject =
    connCount > 0
      ? label('tree.delete.subject.both', { folders: folderCount, connections: connCount })
      : label('tree.delete.subject.folders', { count: folderCount });
  const inside = nested > 0 ? label('tree.delete.including', { count: nested }) : '';
  return {
    title: label('tree.delete.items.title'),
    message: label('security.tree.delete.items', { subject, inside }),
    critical,
    checkboxLabel,
  };
}
