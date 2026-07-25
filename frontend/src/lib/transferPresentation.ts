// Presentation-layer decisions for the Transfers panel, extracted from
// TransferPanel.svelte so they are unit-testable without a component harness.
//
// The backend reports honest operation facts (kind, state, done/total); how
// those facts are labelled, whether a byte-rate applies, and whether an
// operation counts as "scanning" are display decisions that belong here, not
// in the usecase layer (see the removal of batchDisplayKind).
import type { OperationKind } from '../stores/appState';

const KIND_LABEL: Record<OperationKind, string> = {
  upload: 'Upload',
  download: 'Download',
  localcopy: 'Copy',
  delete: 'Delete',
  chmod: 'chmod',
  chown: 'chown',
};

/** Human-readable label for an operation kind, used in the panel row title
 *  and the completion notification. */
export function kindLabel(kind: OperationKind): string {
  return KIND_LABEL[kind] ?? 'Operation';
}

/** Whether an active item of this kind moves bytes and so has a meaningful
 *  transfer rate. Local copies move bytes just like uploads/downloads. */
export function showsRate(kind: OperationKind, state: string): boolean {
  return (kind === 'upload' || kind === 'download' || kind === 'localcopy') && state === 'active';
}

/** An operation is "scanning" — enumerating its sources with no known total
 *  yet — when it is active and its total is not yet known. This holds
 *  regardless of kind, so adding a new kind never requires touching this
 *  predicate. */
export function isScanning(state: string, total: number): boolean {
  return state === 'active' && total <= 0;
}
