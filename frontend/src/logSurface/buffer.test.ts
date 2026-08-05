// Behaviour of the log surface's bounded buffer and its search/export helpers (ADR-015 §1).
import { LogBuffer } from './buffer';
import { searchLines, stepMatch } from './search';
import { exportLines, exportFileName } from './export';

let failures = 0;
function assert(cond: boolean, msg: string) {
  if (!cond) {
    failures++;
    console.error('FAIL:', msg);
  }
}

const enc = (s: string) => new TextEncoder().encode(s);

// --- line splitting --------------------------------------------------------

{
  const b = new LogBuffer();
  b.append(enc('one\ntwo\n'), 'stdout');
  assert(b.snapshot().length === 2, 'two complete lines are two entries');
  assert(b.snapshot()[0].text === 'one', 'first line text');
}

{
  // A chunk boundary is not a line boundary: a trailing fragment waits for its newline instead of
  // being shown as a line that was never written.
  const b = new LogBuffer();
  b.append(enc('par'), 'stdout');
  assert(b.snapshot().length === 0, 'a partial line is not yet a line');
  b.append(enc('tial\n'), 'stdout');
  assert(b.snapshot().length === 1 && b.snapshot()[0].text === 'partial', 'partial line completes');
}

{
  // CRLF output must not leave a stray carriage return at the end of every line.
  const b = new LogBuffer();
  b.append(enc('crlf\r\n'), 'stdout');
  assert(b.snapshot()[0].text === 'crlf', 'trailing CR is stripped');
}

{
  // stdout and stderr interleave; one shared partial would splice a half-line of one into the other.
  const b = new LogBuffer();
  b.append(enc('out-'), 'stdout');
  b.append(enc('err\n'), 'stderr');
  b.append(enc('put\n'), 'stdout');
  const texts = b.snapshot().map((l) => `${l.stream}:${l.text}`);
  assert(texts.join('|') === 'stderr:err|stdout:out-put', `streams stay separate, got ${texts.join('|')}`);
}

{
  // A multi-byte character split across chunks must not become replacement characters.
  const bytes = enc('щука\n');
  const b = new LogBuffer();
  b.append(bytes.slice(0, 3), 'stdout');
  b.append(bytes.slice(3), 'stdout');
  assert(b.snapshot()[0].text === 'щука', `utf-8 split across chunks, got ${b.snapshot()[0]?.text}`);
}

{
  // A producer that never wrote a trailing newline must not lose its last line.
  const b = new LogBuffer();
  b.append(enc('no newline'), 'stdout');
  b.flush();
  assert(b.snapshot().length === 1 && b.snapshot()[0].text === 'no newline', 'flush promotes the partial line');
}

// --- bounds ----------------------------------------------------------------

{
  const b = new LogBuffer(1 << 20, 3);
  for (let i = 0; i < 5; i++) b.append(enc(`line${i}\n`), 'stdout');
  assert(b.snapshot().length === 3, 'line cap holds');
  assert(b.snapshot()[0].text === 'line2', 'the OLDEST lines are the ones dropped');
  assert(b.truncated(), 'truncation is reported, not hidden');
}

{
  const b = new LogBuffer(10, 1000);
  b.append(enc('12345\n67890\nabcde\n'), 'stdout');
  assert(b.truncated(), 'byte cap also reports truncation');
  assert(b.snapshot().length < 3, 'byte cap actually drops lines');
}

{
  const b = new LogBuffer();
  assert(!b.truncated(), 'a fresh buffer has dropped nothing');
  b.append(enc('x\n'), 'stdout');
  b.clear();
  assert(b.snapshot().length === 0 && !b.truncated(), 'clear resets content and the flag');
}

// --- search ----------------------------------------------------------------

{
  const b = new LogBuffer();
  b.append(enc('Alpha\nbeta\nALPHA\n'), 'stdout');
  const lines = b.snapshot();
  assert(searchLines(lines, '', false).length === 0, 'an empty query matches nothing, not everything');
  assert(searchLines(lines, 'alpha', false).length === 2, 'case-insensitive matches both');
  assert(searchLines(lines, 'alpha', true).length === 0, 'case-sensitive matches neither');
  assert(searchLines(lines, 'Alpha', true).length === 1, 'case-sensitive matches the exact one');
}

{
  const hits = [1, 4, 9];
  assert(stepMatch(hits, 0, 1) === 1, 'next match');
  assert(stepMatch(hits, 2, 1) === 0, 'next wraps at the end');
  assert(stepMatch(hits, 0, -1) === 2, 'previous wraps at the start');
  assert(stepMatch([], 0, 1) === -1, 'no matches means no position');
}

// --- export ----------------------------------------------------------------

{
  const b = new LogBuffer();
  b.append(enc('ok\n'), 'stdout');
  b.append(enc('bad\n'), 'stderr');
  const lines = b.snapshot();
  const tagged = exportLines(lines, true);
  assert(tagged.includes('[stderr] bad'), 'stderr is distinguishable in a saved file');
  assert(!exportLines(lines, false).includes('[stderr]'), 'tagging is optional');
  assert(exportLines([], true) === '', 'an empty buffer exports nothing, not a blank line');
}

{
  // A plugin chose the title, so it must not reach a filesystem path unfiltered.
  const name = exportFileName('docker logs: /etc/../nginx');
  assert(!name.includes('/') && !name.includes('..'), `export filename is sanitized, got ${name}`);
  assert(exportFileName('').startsWith('log-'), 'an empty title still yields a filename');
}

if (failures > 0) {
  console.error(`buffer.test: ${failures} failure(s)`);
  process.exit(1);
}
console.log('logSurface buffer/search/export tests passed');
