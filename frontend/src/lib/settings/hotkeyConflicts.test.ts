import { findHotkeyConflict, type SessionHotkeyDraft } from './hotkeyConflicts';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const label = (key: string, vars?: Record<string, string | number>) =>
  vars ? `${vars.first} vs ${vars.second}` : key.split('.').pop() ?? key;

const base: SessionHotkeyDraft = {
  sessionHotkeyCreate: 'Ctrl+Shift+N',
  sessionHotkeyNext: 'Ctrl+Tab',
  sessionHotkeyPrev: 'Ctrl+Shift+Tab',
  sessionHotkeyClose: 'Ctrl+Shift+Q',
  localTerminalHotkey: 'Ctrl+Shift+T',
  terminalSearchHotkey: 'Ctrl+Shift+F',
  multiInputHotkey: 'Alt+M',
  multiInputStopHotkey: 'Alt+Shift+M',
};

assert(findHotkeyConflict(base, label) === '', 'the defaults do not conflict with each other');

// The terminal tools take part in the check: two actions on one chord would fight over every press.
assert(findHotkeyConflict({ ...base, multiInputHotkey: 'ctrl+shift+f' }, label) === 'terminalSearch vs multiInput',
  'search and multi-input on the same chord, in any spelling, are a conflict');
assert(findHotkeyConflict({ ...base, terminalSearchHotkey: 'Ctrl+Shift+T' }, label) === 'newLocalTerminal vs terminalSearch',
  'a terminal tool clashing with an existing hotkey is reported');

assert(findHotkeyConflict({ ...base, multiInputStopHotkey: 'alt+m' }, label) === 'multiInput vs multiInputStop',
  'the stop binding cannot share the chord that starts multi-input');

console.log('hotkeyConflicts.test passed');
