import { runSearch, previewOf, flattenHits } from './searchRun';
import type { LogicalLine } from './bufferText';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

function lines(...texts: string[]): LogicalLine[] {
  return texts.map((text, row) => ({ text, row, segments: [{ row, start: 0, text, cols: null, widths: null, width: 80 }] }));
}

const re = /err/giu;

// Groups keep the order of the sources, which is the panel's terminal order, and skip terminals
// with nothing found.
{
  const out = runSearch(
    [
      { terminalId: 't1', lines: lines('ok', 'err one', 'two err err') },
      { terminalId: 't2', lines: lines('nothing here') },
      { terminalId: 't3', lines: lines('ERR') },
    ],
    re
  );
  assert(out.total === 4, `four matches in total, got ${out.total}`);
  assert(out.groups.map((g) => g.terminalId).join() === 't1,t3', `empty terminals are not listed, got ${out.groups.map((g) => g.terminalId)}`);
  assert(out.groups[0].hits.length === 3, 'two matches on one line are two hits');
  assert(!out.truncated, 'under the cap nothing is truncated');
  const flat = flattenHits(out.groups);
  assert(flat[3].terminalId === 't3', 'flattened hits follow group order, so Enter walks terminal by terminal');
}

// The cap stops the search and says so.
{
  const many = lines(...Array.from({ length: 50 }, () => 'err err'));
  const out = runSearch([{ terminalId: 't', lines: many }, { terminalId: 'u', lines: lines('err') }], re, 7);
  assert(out.total === 7, `results stop at the cap, got ${out.total}`);
  assert(out.truncated, 'a capped search reports that it was cut short');
  assert(out.groups.length === 1, 'terminals after the cap are not searched');
}

// The preview keeps the match and some context, marks cut ends and drops indentation.
{
  const [hit] = runSearch([{ terminalId: 't', lines: lines('        indented err here') }], re).groups[0].hits;
  const p = previewOf(hit);
  assert(p.match === 'err', `the match itself is preserved, got ${p.match}`);
  assert(p.before === 'indented ', `leading indentation is dropped, got ${JSON.stringify(p.before)}`);
  const long = 'x'.repeat(300) + 'err' + 'y'.repeat(500);
  const [longHit] = runSearch([{ terminalId: 't', lines: lines(long) }], re).groups[0].hits;
  const lp = previewOf(longHit);
  assert(lp.before.startsWith('…') && lp.after.endsWith('…'), 'a preview cut from a long line says it was cut');
  assert(lp.before.length + lp.after.length < 250, 'a preview of a long line stays short');
  assert(lp.before.length <= 14, `the lead-in stays short so the match is not cut off the row, got ${lp.before.length}`);
}

console.log('searchRun.test passed');
