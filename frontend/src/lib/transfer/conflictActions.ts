// The conflict-resolution vocabulary, mirroring domain.ConflictAction in Go
// (internal/domain/transfer_conflict.go). These string values are the wire
// contract; keep them in sync with ParseConflictAction.

export type ConflictAction =
  | 'overwrite'
  | 'overwrite_if_newer'
  | 'overwrite_if_different_size'
  | 'overwrite_if_newer_or_different_size'
  | 'rename'
  | 'skip';

// 'ask' is the settings-only sentinel meaning "prompt with the dialog". It is
// never a concrete per-file decision.
export type ExistsDefault = ConflictAction | 'ask';

// CONFLICT_ACTIONS is the ordered list shown in the dialog, matching FileZilla's
// order (minus Resume). Each entry carries a message key rather than a caption:
// the value is the wire contract and must not move, while the words shown to the
// user follow the interface language.
export const CONFLICT_ACTIONS: { value: ConflictAction; labelKey: string }[] = [
  { value: 'overwrite', labelKey: 'transfer.conflict.overwrite' },
  { value: 'overwrite_if_newer', labelKey: 'transfer.conflict.overwrite_if_newer' },
  { value: 'overwrite_if_different_size', labelKey: 'transfer.conflict.overwrite_if_different_size' },
  { value: 'overwrite_if_newer_or_different_size', labelKey: 'transfer.conflict.overwrite_if_newer_or_different_size' },
  { value: 'rename', labelKey: 'transfer.conflict.rename' },
  { value: 'skip', labelKey: 'transfer.conflict.skip' },
];

export function isConflictAction(v: string): v is ConflictAction {
  return CONFLICT_ACTIONS.some((a) => a.value === v);
}

// normalizeExistsDefault coerces a stored settings value ('' | 'ask' | action)
// into an ExistsDefault, defaulting to 'ask'.
export function normalizeExistsDefault(v: string | undefined): ExistsDefault {
  if (v && isConflictAction(v)) return v;
  return 'ask';
}
