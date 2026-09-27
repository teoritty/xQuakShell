// The name a duplicated connection gets: "<name> - copy" in the interface language, numbered when
// that name is already taken, the way a file manager names a copy.

export interface CopyNameTemplates {
  /** Contains {name}, e.g. "{name} - copy". */
  first: string;
  /** Contains {name} and {number}, e.g. "{name} - copy {number}". */
  numbered: string;
}

function fill(template: string, name: string, n?: number): string {
  return template.replace('{name}', name).replace('{number}', String(n ?? ''));
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function templatePattern(template: string): RegExp {
  const source = escapeRegExp(template).replace('\\{name\\}', '(.+)').replace('\\{number\\}', '\\d+');
  return new RegExp(`^${source}$`, 'u');
}

/**
 * The name a copy was made from. Duplicating "web - copy" yields "web - copy 2", not
 * "web - copy - copy": a chain of suffixes says nothing the number does not.
 */
export function copyBaseName(name: string, templates: CopyNameTemplates): string {
  for (const template of [templates.numbered, templates.first]) {
    const match = templatePattern(template).exec(name);
    if (match) return match[1];
  }
  return name;
}

/** The first copy name for source that is not already in taken. */
export function duplicateName(source: string, taken: ReadonlySet<string>, templates: CopyNameTemplates): string {
  const base = copyBaseName(source, templates);
  const first = fill(templates.first, base);
  if (!taken.has(first)) return first;
  for (let n = 2; ; n++) {
    const candidate = fill(templates.numbered, base, n);
    if (!taken.has(candidate)) return candidate;
  }
}
