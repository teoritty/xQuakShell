// How the Sync page names what it lists. The backend sends a record's type, the name the person gave
// the item, and, for a conflict, the field path with the elements it runs through named; this turns
// them into words in the person's language. Record ids, natural keys and "[id=…]" selectors never
// reach the screen: a conflict once read `connection 0922d087-… · users.[id=u-f577…].passAuth`.

type Translate = (key: string, vars?: Record<string, string | number>) => string;

/** The translated word for a key, or '' when the language packs have none - $t returns the key. */
function known(t: Translate, key: string): string {
  const s = t(key);
  return s === key ? '' : s;
}

/** What kind of item a record type holds: "Connection", "Settings: lockout", … */
export function kindLabel(type: string, t: Translate): string {
  if (type.startsWith('settings.')) {
    const section = known(t, `sync.kind.section.${type.slice('settings.'.length)}`);
    return section ? t('sync.kind.settings', { section }) : t('sync.kind.settingsAny');
  }
  return known(t, `sync.kind.${type}`) || t('sync.kind.other');
}

/** An item as a person recognises it: its kind and, when it has one, its name. */
export function itemLabel(type: string, name: string | undefined, t: Translate): string {
  const kind = kindLabel(type, t);
  return name ? t('sync.item.named', { kind, name }) : kind;
}

// Path segments that only say where a field sits - the auth block of a user, a key's blob - and
// would read as noise between the element and the field that actually conflicts.
const structural = new Set(['passAuth', 'keyAuth', 'pluginAuth', 'jumpChain', 'blob', 'identity']);

/**
 * The field a conflict is about, e.g. "User «deploy» · Password". A selector adds the element's
 * name to the part before it; a field without a translation keeps its own name, which beats
 * dropping it.
 */
export function conflictFieldLabel(path: string[], names: string[] | undefined, t: Translate): string {
  const parts: string[] = [];
  path.forEach((seg, i) => {
    if (seg.startsWith('[')) {
      const name = names?.[i];
      if (name && parts.length > 0) parts[parts.length - 1] = t('sync.item.named', { kind: parts[parts.length - 1], name });
      return;
    }
    if (structural.has(seg)) return;
    parts.push(known(t, `sync.field.${seg}`) || seg);
  });
  return parts.join(' · ');
}
