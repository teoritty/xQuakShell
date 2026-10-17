// A form that edits a copy of something sync can change underneath it - a connection, the settings
// - must not write its whole copy back, or every field another device changed while the form was
// open is reverted on every device, with no conflict, because to the merge it is simply a newer
// edit. rebaseDraft is the three-way merge such a form needs before it saves.

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

/**
 * Moves a draft onto a newer version of what it was built from: every field still equal to what
 * the form opened with (`base`) takes the newer value, every field the person changed keeps the
 * person's. Where both changed the same field, the value on screen wins - it is what the person
 * is looking at when they save.
 */
export function rebaseDraft<T extends object>(draft: T, base: T, latest: T): T {
  const out = { ...draft };
  for (const key of Object.keys(latest) as (keyof T)[]) {
    if (same(draft[key], base[key])) out[key] = latest[key];
  }
  return out;
}
