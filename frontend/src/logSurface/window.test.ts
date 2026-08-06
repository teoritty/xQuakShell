// Windowing arithmetic for the log viewer (ADR-015 §1).
//
// The viewer renders only the rows a viewport can show. Getting this wrong is either blank rows
// while scrolling or the whole buffer in the DOM, which is what it was before: a log allowed to
// reach 200 000 lines cannot be a node per line.
import { computeLogWindow, scrollTopForLine, LOG_WINDOW_OVERSCAN } from './window';

let failures = 0;
function assert(cond: boolean, msg: string) {
  if (!cond) {
    failures++;
    console.error('FAIL:', msg);
  }
}

const ROW = 18;

// --- the point of the exercise ---------------------------------------------

{
  // A huge log with a small viewport renders a screenful, not the log.
  const w = computeLogWindow(200000, 0, 360, ROW);
  assert(w.count < 100, `a full buffer must not be fully rendered, count=${w.count}`);
  assert(w.totalHeight === 200000 * ROW, 'the scrollbar still describes the whole log');
}

// --- edges ------------------------------------------------------------------

{
  const w = computeLogWindow(0, 0, 360, ROW);
  assert(w.count === 0 && w.totalHeight === 0, 'an empty log renders nothing');
}

{
  const w = computeLogWindow(1000, 0, 360, 0);
  assert(w.count === 0, 'a zero row height cannot address rows; render nothing rather than divide');
}

{
  // At the top there is nothing above to overscan into.
  const w = computeLogWindow(1000, 0, 360, ROW);
  assert(w.first === 0 && w.topPad === 0, 'the first window starts at the first line');
}

{
  // Scrolled far down: the window follows, and the spacer above matches what is not rendered.
  const w = computeLogWindow(1000, 500 * ROW, 360, ROW);
  assert(w.first === 500 - LOG_WINDOW_OVERSCAN, `window starts an overscan early, got ${w.first}`);
  assert(w.topPad === w.first * ROW, 'the spacer is exactly the height of the skipped rows');
  assert(w.first + w.count <= 1000, 'the window never runs past the end of the buffer');
}

{
  // At the very bottom the window is short rather than reaching past the last line.
  const total = 50;
  const w = computeLogWindow(total, total * ROW, 360, ROW);
  assert(w.first + w.count === total, `the last window ends at the last line, got ${w.first + w.count}`);
}

{
  // A viewport taller than the log renders all of it.
  const w = computeLogWindow(5, 0, 1000, ROW);
  assert(w.first === 0 && w.count === 5, 'a short log is rendered whole');
}

{
  // A negative scrollTop (elastic scrolling on some platforms) must not produce a negative index.
  const w = computeLogWindow(1000, -50, 360, ROW);
  assert(w.first === 0 && w.topPad === 0, 'over-scrolling upwards clamps to the top');
}

// --- jumping to a match -----------------------------------------------------

{
  const top = scrollTopForLine(500, 1000, 360, ROW);
  assert(top > 0 && top < 1000 * ROW, 'a mid-log match scrolls somewhere inside the log');
  const centred = 500 * ROW - (360 - ROW) / 2;
  assert(Math.abs(top - centred) < 1, `the match is centred, got ${top} want ${centred}`);
}

{
  assert(scrollTopForLine(0, 1000, 360, ROW) === 0, 'the first line cannot scroll above the top');
  const max = 1000 * ROW - 360;
  assert(scrollTopForLine(999, 1000, 360, ROW) === max, 'the last line clamps to the bottom');
}

{
  assert(scrollTopForLine(-1, 10, 360, ROW) === 0, 'no match means no scroll');
}

if (failures > 0) {
  console.error(`window.test: ${failures} failure(s)`);
  process.exit(1);
}
console.log('logSurface window tests passed');
