import { DEFAULT_APP_HOTKEYS, hotkeysFromSettings } from './appHotkeys';
import type { AppSettings } from '../api/settings';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

// A vault written before a binding existed sends it empty; the dispatcher must still have one.
const empty = hotkeysFromSettings({} as AppSettings);
for (const [name, value] of Object.entries(empty)) {
  assert(value === DEFAULT_APP_HOTKEYS[name as keyof typeof empty], `empty ${name} falls back to its default, got ${value}`);
}
assert(DEFAULT_APP_HOTKEYS.multiInputStop === 'Alt+Shift+M', 'multi-input has a stop binding by default');
assert(DEFAULT_APP_HOTKEYS.multiInputStop !== DEFAULT_APP_HOTKEYS.multiInput, 'stop is not the chord that starts');

// A saved binding wins over its default, for every field including the stop one.
const saved = hotkeysFromSettings({ multiInputStopHotkey: 'Ctrl+Alt+M', terminalSearchHotkey: 'Ctrl+F' } as AppSettings);
assert(saved.multiInputStop === 'Ctrl+Alt+M', `the saved stop binding is used, got ${saved.multiInputStop}`);
assert(saved.search === 'Ctrl+F', `the saved search binding is used, got ${saved.search}`);

console.log('appHotkeys.test passed');
