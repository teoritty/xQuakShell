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
// order (minus Resume).
export const CONFLICT_ACTIONS: { value: ConflictAction; label: string }[] = [
  { value: 'overwrite', label: 'Overwrite' },
  { value: 'overwrite_if_newer', label: 'Overwrite if source newer' },
  { value: 'overwrite_if_different_size', label: 'Overwrite if different size' },
  { value: 'overwrite_if_newer_or_different_size', label: 'Overwrite if different size or source newer' },
  { value: 'rename', label: 'Rename' },
  { value: 'skip', label: 'Skip' },
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
