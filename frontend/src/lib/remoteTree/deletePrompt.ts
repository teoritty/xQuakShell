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

/**
 * Wording for the delete confirmation, kept out of the component because it is
 * the only thing standing between the user and an irreversible cascade: a
 * folder delete takes its whole subtree with it, so the dialog has to say how
 * many connections that actually is. Pure and separate so those counts can be
 * tested rather than eyeballed in the UI.
 */
export function describeDeleteTargets(
  targets: DeleteTargets,
  folders: Folder[],
  connections: Connection[]
): DeletePrompt {
  const folderCount = targets.folderIds.length;
  const connCount = targets.connectionIds.length;
  const nested = targets.folderIds.reduce(
    (sum, id) => sum + countConnectionsInFolder(id, folders, connections),
    0
  );
  const critical = folderCount + connCount > 1 || nested > 0;
  const checkboxLabel =
    folderCount > 0
      ? 'I understand this will permanently delete all connections inside these folders'
      : 'I understand this will permanently delete the selected connections';

  if (folderCount === 1 && connCount === 0) {
    const name = folders.find((f) => f.id === targets.folderIds[0])?.name ?? '';
    if (nested === 0) {
      return {
        title: 'Delete Folder',
        message: `Are you sure you want to delete "${name}"?`,
        critical,
        checkboxLabel,
      };
    }
    return {
      title: 'Warning: Folder Contains Connections',
      message: `You are about to delete folder "${name}" which contains ${nested} connection(s). This action cannot be undone!`,
      critical,
      checkboxLabel,
    };
  }

  if (folderCount === 0 && connCount === 1) {
    const name = connections.find((c) => c.id === targets.connectionIds[0])?.name ?? '';
    return {
      title: 'Delete Connection',
      message: `Are you sure you want to delete "${name}"?`,
      critical,
      checkboxLabel,
    };
  }

  if (folderCount === 0) {
    return {
      title: 'Delete Multiple Connections',
      message: `You are about to delete ${connCount} connection(s). This action cannot be undone!`,
      critical,
      checkboxLabel,
    };
  }

  const subject =
    connCount > 0
      ? `${folderCount} folder(s) and ${connCount} connection(s)`
      : `${folderCount} folder(s)`;
  const inside = nested > 0 ? `, including ${nested} connection(s) inside those folders` : '';
  return {
    title: 'Delete Multiple Items',
    message: `You are about to delete ${subject}${inside}. This action cannot be undone!`,
    critical,
    checkboxLabel,
  };
}
