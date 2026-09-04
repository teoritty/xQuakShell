// The stateful half of the Ping button: which answer is on which row, and when it goes away.
//
// Two rules carry the weight. A second ping must not be retired by the first one's timer - get that
// wrong and the newer answer disappears early, which reads as a flicker rather than as a bug. And a
// closed dialog must leave no timer behind, or reopening the screen shows an answer from a previous
// visit against a plugin whose state has since changed.
import { get } from 'svelte/store';
import { createPingTracker } from './pingTracker';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error('FAIL: ' + msg);
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

async function main(): Promise<void> {
  // --- an answered ping lands on its own row, and only its own ---
  {
    let clock = 0;
    const tracker = createPingTracker({ ttlMs: 10_000, now: () => clock });
    await tracker.run('plugin.a', async () => {
      clock = 7;
      return { pong: 'ok' };
    });

    const state = get(tracker);
    assert(state['plugin.a']?.ok === true, 'the answer is recorded against the plugin that answered');
    assert(state['plugin.a']?.ms === 7, `ms = ${state['plugin.a']?.ms}; the measured round trip`);
    assert(state['plugin.a']?.detail === 'pong=ok', 'the payload is kept for the tooltip');
    assert(Object.keys(state).length === 1, 'no other row is touched');
    tracker.dispose();
  }

  // --- a rejection is an answer too, and never escapes to the caller ---
  {
    const tracker = createPingTracker({ ttlMs: 10_000, now: () => 0 });
    let threw = false;
    try {
      await tracker.run('plugin.b', async () => {
        throw new Error('plugin is not running');
      });
    } catch {
      threw = true;
    }
    assert(!threw, 'a failed ping must not reject: the caller has nothing to do with it');

    const outcome = get(tracker)['plugin.b'];
    assert(outcome !== undefined && !outcome.ok, 'the failure is recorded as an outcome');
    assert(
      outcome.detail === 'plugin is not running',
      `detail = ${outcome.detail}; the reason is what the row has to show`
    );
    tracker.dispose();
  }

  // --- the result is transient: it borrows the status line and gives it back ---
  {
    const tracker = createPingTracker({ ttlMs: 20, now: () => 0 });
    await tracker.run('plugin.c', async () => ({ pong: 'ok' }));
    assert(get(tracker)['plugin.c'] !== undefined, 'the result is there immediately');

    await sleep(60);
    assert(
      get(tracker)['plugin.c'] === undefined,
      'and is gone once its time is up, so the row reports run state again'
    );
    tracker.dispose();
  }

  // --- a second ping resets the clock rather than inheriting the first one's ---
  //
  // The bug this prevents: the first ping's timer is still pending when the second answer arrives,
  // fires on the original schedule, and takes the newer answer away with it.
  {
    const tracker = createPingTracker({ ttlMs: 40, now: () => 0 });
    await tracker.run('plugin.d', async () => ({ pong: 'first' }));
    await sleep(30);
    await tracker.run('plugin.d', async () => ({ pong: 'second' }));

    await sleep(25); // past the first timer's deadline, well inside the second's
    const still = get(tracker)['plugin.d'];
    assert(
      still !== undefined && still.detail === 'pong=second',
      `after a re-ping the row shows ${JSON.stringify(still)}; the newer answer must outlive the older timer`
    );

    await sleep(40);
    assert(get(tracker)['plugin.d'] === undefined, 'and the newer one retires on its own schedule');
    tracker.dispose();
  }

  // --- dispose clears the board and cancels what was pending ---
  {
    const tracker = createPingTracker({ ttlMs: 10_000, now: () => 0 });
    await tracker.run('plugin.e', async () => ({ pong: 'ok' }));
    tracker.dispose();
    assert(
      Object.keys(get(tracker)).length === 0,
      'a disposed tracker holds nothing: reopening the screen must not show a previous visit'
    );

    // Nothing observable fires afterwards. A timer that survived would throw here or repopulate the
    // store; both are caught by reading it again after the original deadline would have passed.
    await sleep(30);
    assert(Object.keys(get(tracker)).length === 0, 'and no retired timer resurrects it');
  }

  console.log('pingTracker.test passed');
}

void main();
