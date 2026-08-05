// Client-side field validation, mirroring the host's validateFieldValue
// (internal/usecase/plugin_fields.go).
//
// This is a convenience, never a boundary: the host re-checks every value on submit and is the
// only thing that decides what is accepted. What it buys is telling the user which field is wrong
// while they are still looking at it, instead of after a round trip.

export interface ValidatableField {
  id: string;
  label?: string;
  type: string;
  required?: boolean;
  minLength?: number;
  maxLength?: number;
  min?: number | null;
  max?: number | null;
  pattern?: string;
  options?: { value: string; label: string }[];
}

/** Mirrors the host's MaxKeyValuePairs / MaxCodeFieldBytes. */
export const MAX_KEY_VALUE_PAIRS = 64;
export const MAX_CODE_BYTES = 256 * 1024;

/** Returns an empty string when the value is acceptable, or a message naming the problem. */
export function validateFieldValue(field: ValidatableField, value: string): string {
  if (field.required && value.trim() === '') return 'This field is required';
  if (value === '') return '';

  if (field.type === 'checkbox') {
    return value === 'true' || value === 'false' ? '' : 'Must be true or false';
  }
  if (field.type === 'keyValue') return validateKeyValue(value);
  if (field.type === 'code') {
    return value.length > MAX_CODE_BYTES ? 'Content is too large' : '';
  }

  if (field.minLength && value.length < field.minLength) {
    return `Must be at least ${field.minLength} characters`;
  }
  if (field.maxLength && value.length > field.maxLength) {
    return `Must be at most ${field.maxLength} characters`;
  }
  if (field.pattern) {
    let re: RegExp | null = null;
    try {
      re = new RegExp(field.pattern);
    } catch {
      // A pattern the host compiled but this engine will not is not the user's problem, and
      // refusing their input over it would block a form they cannot fix.
      re = null;
    }
    if (re && !re.test(value)) return 'Value has an unexpected format';
  }
  if (field.type === 'number') {
    const n = Number(value);
    if (Number.isNaN(n)) return 'Must be a number';
    if (field.min != null && n < field.min) return `Must be at least ${field.min}`;
    if (field.max != null && n > field.max) return `Must be at most ${field.max}`;
  }
  if (field.type === 'select' && field.options && field.options.length > 0) {
    if (!field.options.some((o) => o.value === value)) return 'Choose one of the listed options';
  }
  return '';
}

/** Parses a keyValue field's stored form: a JSON object of strings, in declaration order. */
export function parseKeyValue(raw: string): { key: string; value: string }[] {
  if (raw.trim() === '') return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return [];
  // Object.entries preserves insertion order for string keys, which is the order the host
  // preserved on its side for the same reason: rows that reshuffle on repaint are unusable.
  return Object.entries(parsed as Record<string, unknown>).map(([key, value]) => ({
    key,
    value: typeof value === 'string' ? value : String(value ?? ''),
  }));
}

/** Renders pairs back to the stored form, dropping rows the user has not named yet. */
export function encodeKeyValue(pairs: { key: string; value: string }[]): string {
  const obj: Record<string, string> = {};
  for (const pair of pairs) {
    if (pair.key.trim() === '') continue;
    obj[pair.key] = pair.value;
  }
  return JSON.stringify(obj);
}

function validateKeyValue(raw: string): string {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return 'Invalid entries';
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return 'Invalid entries';
  const entries = Object.entries(parsed as Record<string, unknown>);
  if (entries.length > MAX_KEY_VALUE_PAIRS) return `At most ${MAX_KEY_VALUE_PAIRS} entries`;
  for (const [key, value] of entries) {
    if (key.trim() === '') return 'Entry names must not be empty';
    if (typeof value !== 'string') return 'Entry values must be text';
    if (hasUnsafeRune(key) || hasUnsafeRune(value)) return 'Entries must not contain control characters';
  }
  return '';
}

/**
 * Control characters and Unicode bidirectional overrides. Refused rather than stripped, matching
 * the host: this value is about to be sent somewhere as data, and silently altering data is worse
 * than refusing it.
 */
function hasUnsafeRune(s: string): boolean {
  for (const ch of s) {
    const code = ch.codePointAt(0) ?? 0;
    if (code < 0x20 || (code >= 0x7f && code <= 0x9f)) return true;
    if ((code >= 0x202a && code <= 0x202e) || (code >= 0x2066 && code <= 0x2069)) return true;
  }
  return false;
}
