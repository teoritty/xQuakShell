import { normalizeHotkey, parseHotkeyEvent } from './hotkeys';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

assert(normalizeHotkey('') === '', 'empty input yields empty string');
assert(normalizeHotkey('control+shift+n') === 'Ctrl+Shift+N', 'control alias normalizes to Ctrl, single-char key uppercased');
assert(normalizeHotkey('ctrl+n') === 'Ctrl+N', 'ctrl alias normalizes to Ctrl');
assert(normalizeHotkey('shift+meta+ctrl+alt+n') === 'Ctrl+Meta+Alt+Shift+N', 'modifiers reordered to Ctrl,Meta,Alt,Shift regardless of input order');
assert(normalizeHotkey('cmd+n') === 'Meta+N', 'cmd alias normalizes to Meta');
assert(normalizeHotkey('win+n') === 'Meta+N', 'win alias normalizes to Meta');
assert(normalizeHotkey('option+n') === 'Alt+N', 'option alias normalizes to Alt');
assert(normalizeHotkey('space') === 'Space', 'multi-char key word "space" is capitalized');
assert(normalizeHotkey('ctrl+f1') === 'Ctrl+F1', 'multi-char non-modifier key gets capitalized first letter, rest untouched');
assert(normalizeHotkey('  ctrl  +  n  ') === 'Ctrl+N', 'surrounding whitespace on parts is trimmed');
assert(normalizeHotkey('ctrl') === 'Ctrl', 'modifier-only input yields just the modifier');

// parseHotkeyEvent reads ctrlKey/metaKey/altKey/shiftKey/key off an event-shaped object.
function fakeEvent(init: Partial<{ ctrlKey: boolean; metaKey: boolean; altKey: boolean; shiftKey: boolean; key: string }>): KeyboardEvent {
  return {
    ctrlKey: false,
    metaKey: false,
    altKey: false,
    shiftKey: false,
    key: '',
    ...init,
  } as KeyboardEvent;
}

assert(parseHotkeyEvent(fakeEvent({ ctrlKey: true, shiftKey: true, key: 'n' })) === 'Ctrl+Shift+N', 'ctrl+shift+n key event normalizes correctly');
assert(parseHotkeyEvent(fakeEvent({ key: 'Control' })) === '', 'a lone modifier keydown (Control) produces no key part');
assert(parseHotkeyEvent(fakeEvent({ ctrlKey: true, key: ' ' })) === 'Ctrl+Space', 'space key is rendered as Space');
assert(parseHotkeyEvent(fakeEvent({ key: 'F5' })) === 'F5', 'multi-char function key passed through and normalized');
assert(parseHotkeyEvent(fakeEvent({ metaKey: true, altKey: true, key: 'a' })) === 'Meta+Alt+A', 'meta+alt+a key event normalizes correctly');

console.log('hotkeys.test.ts: all passed');
