import {
  RECOVERY_ACKNOWLEDGE_SECONDS,
  acknowledgeLabel,
  isAcknowledgeEnabled,
  tickCountdown,
} from './recoveryCountdown';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function run() {
  // The delay exists so nobody clicks Done reflexively on the one screen that shows the key. A
  // value small enough to click through by accident defeats the point of having it at all.
  assert(
    RECOVERY_ACKNOWLEDGE_SECONDS >= 10,
    'the acknowledge delay is short enough to click through without reading the screen',
  );

  // The counter walks down to zero and stops there. One tick past the end must not render
  // "Done (-1)", and must not push the button back into a disabled state.
  {
    let left = RECOVERY_ACKNOWLEDGE_SECONDS;
    const seen: number[] = [left];
    for (let i = 0; i < RECOVERY_ACKNOWLEDGE_SECONDS + 3; i++) {
      left = tickCountdown(left);
      seen.push(left);
    }
    assert(seen[1] === RECOVERY_ACKNOWLEDGE_SECONDS - 1, 'the first tick takes one second off');
    assert(
      seen[RECOVERY_ACKNOWLEDGE_SECONDS] === 0,
      `the counter reaches zero after ${RECOVERY_ACKNOWLEDGE_SECONDS} ticks, got ${seen[RECOVERY_ACKNOWLEDGE_SECONDS]}`,
    );
    assert(seen.every((n) => n >= 0), 'the counter never goes negative');
    assert(left === 0, 'extra ticks past the end leave it at zero');
  }

  // Done is disabled for the whole countdown and enabled the moment it ends. Enabling it one tick
  // early would let the last second be clicked through, which is the second people actually use.
  {
    for (let n = RECOVERY_ACKNOWLEDGE_SECONDS; n >= 1; n--) {
      assert(!isAcknowledgeEnabled(n), `Done is still disabled at ${n} seconds left`);
    }
    assert(isAcknowledgeEnabled(0), 'Done is enabled at zero');
    assert(isAcknowledgeEnabled(-1), 'a counter that overshot still enables Done');
  }

  // A broken timer must not trap someone in a dialog with no other way out.
  {
    assert(isAcknowledgeEnabled(Number.NaN), 'NaN enables Done rather than blocking forever');
    assert(isAcknowledgeEnabled(Number.POSITIVE_INFINITY), 'Infinity enables Done rather than blocking forever');
    assert(tickCountdown(Number.NaN) === 0, 'ticking NaN lands on zero');
  }

  // The label counts down inside the button, and loses the counter once it is done.
  {
    assert(acknowledgeLabel('Done', 15) === 'Done (15)', 'the label carries the seconds left');
    assert(acknowledgeLabel('Done', 1) === 'Done (1)', 'the last second is still shown');
    assert(acknowledgeLabel('Done', 0) === 'Done', 'the counter disappears when the wait is over');
    assert(acknowledgeLabel('Готово', 9) === 'Готово (9)', 'the label is whatever the caller passes');
  }

  console.log('recoveryCountdown.test passed');
}

run();
