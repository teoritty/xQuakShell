export type MessageVars = Record<string, string | number>;

/**
 * Substitutes `{name}` placeholders in a translated string.
 *
 * Substitution is a plain string replace producing a plain string: the result is interpolated into
 * markup as text, never as HTML, so a translation pack cannot inject an element by writing one.
 * That property is what keeps an untrusted pack on disk from being a scripting vector, and it is
 * why no branch of this file builds markup.
 *
 * A placeholder with no matching variable is left standing rather than blanked. A translator
 * looking at "Copied {count} of {tota}" can see the typo; a translator looking at "Copied 3 of "
 * cannot.
 */
export function interpolate(template: string, vars?: MessageVars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
    name in vars ? String(vars[name]) : whole,
  );
}

/**
 * Picks the plural form for a count, using the locale's own rules.
 *
 * English needs two forms and Russian needs three, chosen by rules no amount of `count === 1`
 * covers: Russian uses "файл" for 1, 21 and 101, "файла" for 2 to 4, and "файлов" for 5 to 20. The
 * platform already knows all of this, so the key carries `_one` / `_few` / `_many` / `_other`
 * suffixes and Intl decides which one applies.
 *
 * A locale whose category is missing from the pack falls through to `_other`, then to the bare key,
 * so a partially translated plural still renders a sentence.
 */
export function pluralKey(
  messages: Record<string, string>,
  key: string,
  locale: string,
  count: number,
): string {
  const category = pluralCategory(locale, count);
  for (const candidate of [`${key}_${category}`, `${key}_other`, key]) {
    if (candidate in messages) return candidate;
  }
  return key;
}

function pluralCategory(locale: string, count: number): string {
  try {
    return new Intl.PluralRules(locale).select(count);
  } catch {
    // An unrecognised locale tag is not worth failing a render over: English rules are the same
    // ones the untranslated fallback text was written for.
    return new Intl.PluralRules('en').select(count);
  }
}
