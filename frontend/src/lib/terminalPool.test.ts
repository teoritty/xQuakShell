import assert from 'node:assert/strict';
import type { Terminal } from '@xterm/xterm';
import type { FitAddon } from '@xterm/addon-fit';
import { getPooledTerminal, setPooledTerminal, transferPooledTerminal, type PooledTerminal } from './terminalPool';

// Only what the pool touches: write for the banner. A real xterm needs a DOM this runner lacks.
function fakePooled(): { pooled: PooledTerminal; written: string[] } {
  const written: string[] = [];
  const term = { write: (s: string) => written.push(s), dispose: () => {} } as unknown as Terminal;
  const host = { parentNode: null } as unknown as HTMLDivElement;
  return { pooled: { term, fitAddon: {} as FitAddon, host, baseFontSize: 14 }, written };
}

// The handover moves the SAME terminal - that is what keeps scrollback and screen - and marks it so
// the next mount tells the new PTY its size.
{
  const { pooled, written } = fakePooled();
  setPooledTerminal('old-1', pooled);

  assert.equal(transferPooledTerminal('old-1', 'new-1', '--banner--'), true, 'a pooled terminal is handed over');
  assert.equal(getPooledTerminal('old-1'), undefined, 'the old id no longer owns it, so closing the old session cannot dispose it');
  const moved = getPooledTerminal('new-1');
  assert.equal(moved?.term, pooled.term, 'the new id gets the same xterm instance, not a copy');
  assert.equal(moved?.sizeUnsent, true, 'the handed-over terminal must announce its size to the new PTY');
  assert.deepEqual(written, ['--banner--'], 'the banner separates the old output from the new shell');
}

// Nothing to hand over, or a new session that already has its own terminal: refuse, touch nothing.
{
  assert.equal(transferPooledTerminal('never-pooled', 'new-2', 'x'), false, 'no terminal, no handover');
  assert.equal(getPooledTerminal('new-2'), undefined, 'a refused handover creates nothing');

  const a = fakePooled();
  const b = fakePooled();
  setPooledTerminal('old-3', a.pooled);
  setPooledTerminal('new-3', b.pooled);
  assert.equal(transferPooledTerminal('old-3', 'new-3', 'x'), false, 'an occupied target is never overwritten');
  assert.equal(getPooledTerminal('new-3')?.term, b.pooled.term, 'the target keeps its own terminal');
  assert.equal(getPooledTerminal('old-3')?.term, a.pooled.term, 'the source keeps its terminal, to be disposed with its session');
  assert.deepEqual(a.written, [], 'a refused handover writes no banner');
}

console.log('terminalPool.test passed');
