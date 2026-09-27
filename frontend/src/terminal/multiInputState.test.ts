import {
  MULTI_INPUT_OFF,
  openPicker,
  toggleMember,
  setDraft,
  confirmPicker,
  cancelPicker,
  pruneClosed,
  mirrorTargets,
  isChosen,
  type MultiInputState,
} from './multiInputState';
import { numberTerminals, terminalForDigit } from './terminalOrder';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const active = (members: string[]): MultiInputState => ({ mode: 'active', members });

// Opening from off preselects the terminal being typed in; from active it edits the group.
{
  const fresh = openPicker(MULTI_INPUT_OFF, 'a');
  assert(fresh.mode === 'selecting' && fresh.draft.join() === 'a', `picker starts with the focused terminal, got ${JSON.stringify(fresh)}`);
  const none = openPicker(MULTI_INPUT_OFF, null);
  assert(none.mode === 'selecting' && none.draft.length === 0, 'with nothing focused the picker starts empty');
  const edit = openPicker(active(['a', 'b']), 'c');
  assert(edit.mode === 'selecting' && edit.draft.join() === 'a,b', 'reopening edits the current group, not the focused tab');
}

// Toggling, all and none only act while picking.
{
  let s = openPicker(MULTI_INPUT_OFF, 'a');
  s = toggleMember(s, 'b');
  s = toggleMember(s, 'a');
  assert(s.mode === 'selecting' && s.draft.join() === 'b', `toggle adds and removes, got ${JSON.stringify(s)}`);
  assert(toggleMember(active(['a', 'b']), 'c').mode === 'active', 'toggle does nothing outside the picker');
  assert(setDraft(MULTI_INPUT_OFF, ['a']) === MULTI_INPUT_OFF, 'select-all does nothing outside the picker');
}

// Enter starts a group of two or more; fewer is how the picker turns multi-input off.
{
  const two = confirmPicker(setDraft(openPicker(MULTI_INPUT_OFF, null), ['a', 'b']));
  assert(two.mode === 'active' && two.members.join() === 'a,b', `two chosen start a group, got ${JSON.stringify(two)}`);
  const one = confirmPicker(openPicker(MULTI_INPUT_OFF, 'a'));
  assert(one.mode === 'off', 'one chosen is not a group: Enter turns multi-input off');
  const cleared = confirmPicker(setDraft(openPicker(active(['a', 'b']), null), []));
  assert(cleared.mode === 'off', 'emptying the group and pressing Enter stops multi-input');
}

// Escape restores exactly what was there before the picker opened.
{
  const edited = toggleMember(openPicker(active(['a', 'b']), null), 'c');
  const back = cancelPicker(edited);
  assert(back.mode === 'active' && back.members.join() === 'a,b', `cancel restores the previous group, got ${JSON.stringify(back)}`);
  assert(cancelPicker(toggleMember(openPicker(MULTI_INPUT_OFF, 'a'), 'b')).mode === 'off', 'cancel from off returns to off');
}

// Closed terminals leave; a group of one ends.
{
  const s = pruneClosed(active(['a', 'b', 'c']), new Set(['a', 'c']));
  assert(s.mode === 'active' && s.members.join() === 'a,c', `a closed member leaves the group, got ${JSON.stringify(s)}`);
  assert(pruneClosed(active(['a', 'b']), new Set(['a'])).mode === 'off', 'a group left with one member ends');
  const same = active(['a', 'b']);
  assert(pruneClosed(same, new Set(['a', 'b', 'z'])) === same, 'nothing closed returns the same state object');
  const picking = pruneClosed(openPicker(active(['a', 'b']), null), new Set(['b']));
  assert(picking.mode === 'selecting' && picking.draft.join() === 'b' && picking.previous.join() === 'b',
    'a closed terminal leaves the draft and the group cancel would restore');
}

// Mirroring reaches the other members only, and only from a member.
{
  const s = active(['a', 'b', 'c']);
  assert(mirrorTargets(s, 'b').join() === 'a,c', `input to b goes to a and c, got ${mirrorTargets(s, 'b')}`);
  assert(mirrorTargets(s, 'z').length === 0, 'input to a terminal outside the group goes nowhere else');
  assert(mirrorTargets(openPicker(s, null), 'a').length === 0, 'nothing is mirrored while the picker is open');
  assert(isChosen(s, 'a') && !isChosen(s, 'z') && !isChosen(MULTI_INPUT_OFF, 'a'), 'isChosen follows the group');
}

// Numbering follows the screen: tile by tile, tab by tab, skipping tabs that are not terminals.
{
  const tiles = [
    { tabs: ['s1', 'vnc', 's2'], activeTabId: 's2' },
    { tabs: ['local'], activeTabId: 'local' },
  ];
  const numbered = numberTerminals(tiles, new Set(['s1', 's2', 'local']));
  assert(numbered.map((n) => `${n.number}:${n.id}`).join() === '1:s1,2:s2,3:local',
    `reading order with non-terminals skipped, got ${numbered.map((n) => `${n.number}:${n.id}`)}`);
  assert(!numbered[0].visible && numbered[1].visible, 'only the active tab of a tile is visible');
  assert(terminalForDigit(numbered, '3') === 'local', 'digit 3 picks the third terminal');
  assert(terminalForDigit(numbered, '4') === null, 'a digit past the last terminal picks nothing');
  assert(terminalForDigit(numbered, 'x') === null, 'a letter is not a digit');
  const ten = numberTerminals([{ tabs: Array.from({ length: 10 }, (_, i) => `t${i}`), activeTabId: '' }],
    new Set(Array.from({ length: 10 }, (_, i) => `t${i}`)));
  assert(terminalForDigit(ten, '0') === 't9', '0 picks the tenth terminal');
}

console.log('multiInputState.test passed');
