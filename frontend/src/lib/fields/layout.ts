// Field layout rules, shared by every renderer of the declarative field schema.
//
// Extracted from PluginConnectionFields.svelte when dialogs and node details became a second and
// third consumer (ADR-015). Only the pure rules moved: the connection editor's markup is mostly
// about vault-stored secrets, which a dialog forbids outright, so merging the two components would
// carry a concern into a place that must not have it. What must NOT diverge is how fields are
// ordered, hidden and packed into rows — that is here, and it is testable without a DOM.

export type FieldWidth = 'full' | 'half' | 'third' | undefined;

export interface LayoutField {
  id: string;
  type: string;
  width?: FieldWidth;
  order?: number;
  dependsOn?: string;
}

export type FieldRow<F extends LayoutField> =
  | { kind: 'row'; fields: F[] }
  | { kind: 'single'; field: F };

/**
 * Mirrors the host's IsFieldVisible (internal/domain/plugin/fields.go): a field with a dependency
 * appears only while that dependency is truthy. Hidden fields are never validated and are cleared
 * on save, so a renderer that disagreed with the host here would show a field whose value the host
 * then silently discards.
 */
export function isFieldVisible(field: LayoutField, values: Record<string, unknown>): boolean {
  if (!field.dependsOn) return true;
  return !!values[field.dependsOn];
}

/** Ascending by `order`, stable for equal values. */
export function sortByOrder<F extends LayoutField>(fields: F[]): F[] {
  return [...fields].sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
}

/**
 * Packs visible fields into rows by declared width: two halves or three thirds share a row, and
 * anything full-width — or a checkbox, whose label is its content — takes its own.
 *
 * A partial run is flushed rather than padded, so a half followed by a full does not leave a gap
 * the user reads as a missing field.
 */
export function groupFieldsIntoRows<F extends LayoutField>(
  fields: F[],
  values: Record<string, unknown>
): FieldRow<F>[] {
  const rows: FieldRow<F>[] = [];
  let currentRow: F[] = [];
  let currentWidth: 'half' | 'third' | null = null;

  function flushRow() {
    if (currentRow.length > 0) {
      rows.push({ kind: 'row', fields: currentRow });
      currentRow = [];
      currentWidth = null;
    }
  }

  for (const field of fields) {
    if (!isFieldVisible(field, values)) continue;

    const w = field.width;
    if (field.type === 'checkbox' || field.type === 'code' || field.type === 'keyValue' || w === 'full' || !w) {
      // code and keyValue are always full width: one is a scrolling block and the other a stack of
      // rows, and neither is legible in a half-width column.
      flushRow();
      rows.push({ kind: 'single', field });
      continue;
    }

    if (w === 'half') {
      if (currentWidth === 'half' && currentRow.length < 2) {
        currentRow.push(field);
      } else {
        flushRow();
        currentWidth = 'half';
        currentRow = [field];
      }
      if (currentRow.length === 2) flushRow();
    } else if (w === 'third') {
      if (currentWidth === 'third' && currentRow.length < 3) {
        currentRow.push(field);
      } else {
        flushRow();
        currentWidth = 'third';
        currentRow = [field];
      }
      if (currentRow.length === 3) flushRow();
    }
  }
  flushRow();
  return rows;
}
