// The shared field rules: validation mirroring the host, and the layout the two renderers share.
import { validateFieldValue, parseKeyValue, encodeKeyValue, MAX_KEY_VALUE_PAIRS } from './validate';
import { groupFieldsIntoRows, isFieldVisible, sortByOrder, type LayoutField } from './layout';

let failures = 0;
function assert(cond: boolean, msg: string) {
  if (!cond) {
    failures++;
    console.error('FAIL:', msg);
  }
}

const text = (extra: Record<string, unknown> = {}) => ({ id: 'f', type: 'text', ...extra }) as any;

// --- validation ------------------------------------------------------------

assert(validateFieldValue(text({ required: true }), '') === 'This field is required', 'required');
assert(validateFieldValue(text({ required: true }), '   ') !== '', 'whitespace is not a value');
assert(validateFieldValue(text(), '') === '', 'an optional empty value is fine');
assert(validateFieldValue(text({ minLength: 3 }), 'ab') !== '', 'minLength');
assert(validateFieldValue(text({ maxLength: 3 }), 'abcd') !== '', 'maxLength');
assert(validateFieldValue(text({ pattern: '^[a-z]+$' }), 'ABC') !== '', 'pattern rejects');
assert(validateFieldValue(text({ pattern: '^[a-z]+$' }), 'abc') === '', 'pattern accepts');

// A pattern this engine cannot compile is not the user's problem: refusing their input over it
// would block a form they have no way to fix.
assert(validateFieldValue(text({ pattern: '(?<broken' }), 'anything') === '', 'uncompilable pattern is ignored');

assert(validateFieldValue({ id: 'n', type: 'number', min: 1, max: 10 } as any, '11') !== '', 'number max');
assert(validateFieldValue({ id: 'n', type: 'number', min: 1, max: 10 } as any, '5') === '', 'number in range');
assert(validateFieldValue({ id: 'n', type: 'number' } as any, 'abc') !== '', 'number format');

assert(validateFieldValue({ id: 'c', type: 'checkbox' } as any, 'yes') !== '', 'checkbox is true/false');
assert(validateFieldValue({ id: 'c', type: 'checkbox' } as any, 'true') === '', 'checkbox true');

const sel = { id: 's', type: 'select', options: [{ value: 'a', label: 'A' }] } as any;
assert(validateFieldValue(sel, 'b') !== '', 'select rejects an unlisted value');
assert(validateFieldValue(sel, 'a') === '', 'select accepts a listed value');

// --- keyValue --------------------------------------------------------------

const kv = { id: 'k', type: 'keyValue' } as any;
assert(validateFieldValue(kv, '{"a":"1"}') === '', 'a valid object passes');
assert(validateFieldValue(kv, '["a"]') !== '', 'an array is not an entry list');
assert(validateFieldValue(kv, '{"":"v"}') !== '', 'an empty entry name is refused');
assert(validateFieldValue(kv, '{"a":5}') !== '', 'a non-string value is refused rather than coerced');
// Built via JSON.stringify so the unsafe rune reaches the validator as a proper JSON escape.
// A raw control character inside a JSON string is invalid JSON, so a literal one would be caught
// by the parser and the rune check — the thing under test — would never run.
assert(validateFieldValue(kv, JSON.stringify({ ["a" + String.fromCharCode(7)]: "v" })) !== '', 'a control character is refused');
assert(validateFieldValue(kv, JSON.stringify({ a: "v" + String.fromCharCode(0x202e) })) !== '', 'a bidi override is refused');

{
  const many: Record<string, string> = {};
  for (let i = 0; i <= MAX_KEY_VALUE_PAIRS; i++) many[`k${i}`] = 'v';
  assert(validateFieldValue(kv, JSON.stringify(many)) !== '', 'too many entries are refused');
}

{
  const pairs = parseKeyValue('{"zulu":"1","alpha":"2"}');
  assert(pairs.length === 2 && pairs[0].key === 'zulu', 'parse preserves declaration order');
  assert(parseKeyValue('not json').length === 0, 'unparseable input yields no rows, not a crash');
  const round = parseKeyValue(encodeKeyValue(pairs));
  assert(round.length === 2 && round[1].value === '2', 'round trip');
  assert(encodeKeyValue([{ key: '  ', value: 'x' }]) === '{}', 'an unnamed row is dropped on encode');
}

// --- layout ----------------------------------------------------------------

const f = (id: string, width?: string, type = 'text', dependsOn?: string): LayoutField =>
  ({ id, type, width, dependsOn }) as LayoutField;

{
  const rows = groupFieldsIntoRows([f('a', 'half'), f('b', 'half'), f('c', 'half')], {});
  assert(rows.length === 2, 'two halves share a row, the third starts a new one');
  assert(rows[0].kind === 'row' && rows[0].fields.length === 2, 'first row holds both halves');
}

{
  const rows = groupFieldsIntoRows([f('a', 'half'), f('b', 'full')], {});
  // A partial run is flushed rather than padded: a gap reads as a missing field.
  assert(rows.length === 2 && rows[0].kind === 'row' && rows[0].fields.length === 1, 'partial run is flushed');
}

{
  const rows = groupFieldsIntoRows([f('a', 'half'), f('cb', 'half', 'checkbox')], {});
  assert(rows[1].kind === 'single', 'a checkbox takes its own row whatever its width says');
}

{
  const rows = groupFieldsIntoRows([f('code', 'half', 'code'), f('kv', 'half', 'keyValue')], {});
  assert(rows.every((r) => r.kind === 'single'), 'code and keyValue are always full width');
}

{
  const hidden = f('b', 'full', 'text', 'a');
  assert(!isFieldVisible(hidden, {}), 'a field whose dependency is unset is hidden');
  assert(isFieldVisible(hidden, { a: 'on' }), 'a field whose dependency is truthy is shown');
  const rows = groupFieldsIntoRows([f('a', 'full'), hidden], {});
  assert(rows.length === 1, 'hidden fields are not laid out');
}

{
  const sorted = sortByOrder([
    { id: 'b', type: 'text', order: 2 },
    { id: 'a', type: 'text', order: 1 },
  ]);
  assert(sorted[0].id === 'a', 'sortByOrder is ascending');
}

if (failures > 0) {
  console.error(`fields validate.test: ${failures} failure(s)`);
  process.exit(1);
}
console.log('fields validate/layout tests passed');
