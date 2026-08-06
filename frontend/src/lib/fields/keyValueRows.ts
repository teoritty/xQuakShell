// The editable-row model behind a keyValue field (ADR-015).
//
// The field's stored form is a JSON object, which is what the plugin receives and what preserves
// declaration order. An object cannot represent what an editor needs, though: a row whose name is
// momentarily empty because the user is retyping it, or two rows briefly sharing a name. Encoding
// on every keystroke made both disappear — clearing a name deleted the row and its value with it,
// and a duplicate name silently swallowed the other row's value.
//
// So the editor holds rows, and the object is what it renders down to. The rules live here, out of
// the component, because losing someone's input is a logic bug worth testing without a DOM.

export interface KeyValueRow {
  /** Stable across edits, so a row keeps its identity while its name changes. */
  id: number;
  key: string;
  value: string;
}

let nextRowID = 1;

/** Parses the stored JSON object into rows, preserving order. */
export function rowsFromValue(raw: string): KeyValueRow[] {
  if (raw.trim() === '') return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return [];
  // Object.entries preserves insertion order for string keys, which is the order the host preserved
  // on its side for the same reason: rows that reshuffle on repaint are unusable.
  return Object.entries(parsed as Record<string, unknown>).map(([key, value]) => ({
    id: nextRowID++,
    key,
    value: typeof value === 'string' ? value : String(value ?? ''),
  }));
}

/**
 * Renders rows back to the stored form.
 *
 * A row with no name yet contributes nothing — there is no key to store it under — but it stays in
 * the editor, which is the whole difference from the old encode-on-every-keystroke behaviour. A
 * duplicated name keeps the first row's value, matching what JSON.parse would do to the object,
 * so what the plugin receives is what a reader of the JSON would expect.
 */
export function valueFromRows(rows: readonly KeyValueRow[]): string {
  const obj: Record<string, string> = {};
  for (const row of rows) {
    const key = row.key.trim();
    if (key === '') continue;
    if (key in obj) continue;
    obj[key] = row.value;
  }
  return JSON.stringify(obj);
}

/**
 * Names that cannot be stored as written, reported per row so the editor can mark them.
 *
 * Blank and duplicate names are the two ways a row silently disappeared before. Saying so is what
 * makes the difference visible: the row is still there, it just is not part of the value yet.
 */
export function rowProblems(rows: readonly KeyValueRow[]): Map<number, string> {
  const problems = new Map<number, string>();
  const seen = new Set<string>();
  for (const row of rows) {
    const key = row.key.trim();
    if (key === '') {
      if (row.value !== '') problems.set(row.id, 'Name this entry to keep it');
      continue;
    }
    if (seen.has(key)) {
      problems.set(row.id, 'Duplicate name');
      continue;
    }
    seen.add(key);
  }
  return problems;
}

/** A new empty row. Empty rather than pre-named: a placeholder name the user must remember to
 * replace is how "key3" ends up in somebody's container labels. */
export function newRow(): KeyValueRow {
  return { id: nextRowID++, key: '', value: '' };
}

/**
 * Whether the rows describe the same value as `raw`.
 *
 * The editor re-reads its rows from the prop only when the value changed underneath it — a
 * different node selected, a snapshot pushed by the plugin — and not when the change is the echo
 * of what it just emitted. Without this the rows would be rebuilt on every keystroke and a blank
 * name would vanish again by another route.
 */
export function rowsMatchValue(rows: readonly KeyValueRow[], raw: string): boolean {
  return valueFromRows(rows) === (raw.trim() === '' ? '{}' : normalizeValue(raw));
}

function normalizeValue(raw: string): string {
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return '{}';
    const obj: Record<string, string> = {};
    for (const [key, value] of Object.entries(parsed as Record<string, unknown>)) {
      obj[key] = typeof value === 'string' ? value : String(value ?? '');
    }
    return JSON.stringify(obj);
  } catch {
    return '{}';
  }
}
