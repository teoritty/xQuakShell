// Naming a newly created entry so it does not collide with an existing one.

/**
 * `base`, or the first free `base (n)`.
 *
 * Numbering starts at 2 because the unsuffixed name is the first: "New Folder",
 * "New Folder (2)", "New Folder (3)". Both panes had this inline and neither had
 * a test, so the off-by-one that would make the second folder "(1)" was one
 * careless edit away.
 */
export function uniqueName(existingNames: readonly string[], base: string): string {
  const taken = new Set(existingNames);
  if (!taken.has(base)) return base;
  let i = 2;
  while (taken.has(`${base} (${i})`)) i++;
  return `${base} (${i})`;
}
