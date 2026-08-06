// The editable-row model behind a keyValue field (ADR-015).
//
// The regressions this file exists for, both of which silently destroyed input: clearing a row's
// name to retype it deleted the row and its value, and two rows briefly sharing a name merged into
// one. Both came from re-deriving the rows from the encoded object on every keystroke.
import {
  newRow,
  rowProblems,
  rowsFromValue,
  rowsMatchValue,
  valueFromRows,
  type KeyValueRow,
} from './keyValueRows';

let failures = 0;
function assert(cond: boolean, msg: string) {
  if (!cond) {
    failures++;
    console.error('FAIL:', msg);
  }
}

function rows(...pairs: [string, string][]): KeyValueRow[] {
  return pairs.map(([key, value]) => ({ ...newRow(), key, value }));
}

// --- the two regressions -----------------------------------------------------

{
  // Mid-rename: the name is empty for a keystroke or two. The row must survive it, with its value.
  const editing = rows(['', 'nginx']);
  assert(rowProblems(editing).size === 1, 'an unnamed row with a value is flagged, not deleted');
  assert(editing[0].value === 'nginx', 'the value is still there while the name is being typed');
  assert(valueFromRows(editing) === '{}', 'an unnamed row contributes nothing to the stored value');

  editing[0].key = 'image';
  assert(valueFromRows(editing) === '{"image":"nginx"}', 'naming the row puts it back in the value');
}

{
  // Two rows sharing a name: the second is flagged rather than swallowing the first.
  const duplicated = rows(['env', 'prod'], ['env', 'staging']);
  const problems = rowProblems(duplicated);
  assert(problems.size === 1, `exactly one row is flagged, got ${problems.size}`);
  assert(problems.has(duplicated[1].id), 'the second occurrence is the flagged one');
  assert(!problems.has(duplicated[0].id), 'the first occurrence is fine');
  assert(
    valueFromRows(duplicated) === '{"env":"prod"}',
    `the first value wins, matching JSON.parse: ${valueFromRows(duplicated)}`
  );
  assert(duplicated.length === 2, 'neither row was removed');
}

// --- parsing and rendering ----------------------------------------------------

{
  const parsed = rowsFromValue('{"zulu":"1","alpha":"2"}');
  assert(parsed.length === 2 && parsed[0].key === 'zulu', 'declaration order is preserved');
  assert(parsed[0].id !== parsed[1].id, 'each row gets its own identity');
  assert(valueFromRows(parsed) === '{"zulu":"1","alpha":"2"}', 'round trip preserves order');
}

{
  assert(rowsFromValue('not json').length === 0, 'unparseable input yields no rows, not a crash');
  assert(rowsFromValue('[1,2]').length === 0, 'an array is not a key/value object');
  assert(rowsFromValue('').length === 0, 'an empty value is no rows');
  assert(rowsFromValue('   ').length === 0, 'whitespace is no rows');
}

{
  // A non-string value in the stored object is coerced rather than dropped: the field's own
  // validator refuses it, and showing the user what is actually stored is what lets them fix it.
  const parsed = rowsFromValue('{"port":8080}');
  assert(parsed.length === 1 && parsed[0].value === '8080', 'a non-string value is shown as text');
}

{
  const trimmed = rows([' spaced ', 'v']);
  assert(valueFromRows(trimmed) === '{"spaced":"v"}', 'names are trimmed on the way out');
}

{
  assert(valueFromRows([]) === '{}', 'no rows is an empty object, not an empty string');
  const blank = newRow();
  assert(blank.key === '' && blank.value === '', 'a new row is empty rather than pre-named');
  assert(rowProblems([blank]).size === 0, 'an untouched new row is not an error yet');
}

// --- re-reading from the prop --------------------------------------------------

{
  // The editor re-reads only when the value changed underneath it. If the incoming value is the
  // echo of what these rows render to, the rows must be left alone — rebuilding them is how a
  // half-typed name disappears by another route.
  const current = rows(['a', '1']);
  assert(rowsMatchValue(current, '{"a":"1"}'), 'an echo of the current rows is recognised');
  assert(!rowsMatchValue(current, '{"a":"2"}'), 'a changed value is not an echo');
  assert(!rowsMatchValue(current, '{}'), 'a cleared value is not an echo');

  const midEdit = rows(['', 'nginx']);
  assert(
    rowsMatchValue(midEdit, '{}'),
    'a row being renamed renders to {}, so the echo of that must not rebuild it'
  );
  assert(rowsMatchValue([], ''), 'no rows matches an empty value');
  assert(rowsMatchValue([], 'not json'), 'unparseable input is treated as empty, not as a change');
}

if (failures > 0) {
  console.error(`keyValueRows.test: ${failures} failure(s)`);
  process.exit(1);
}
console.log('keyValueRows tests passed');
