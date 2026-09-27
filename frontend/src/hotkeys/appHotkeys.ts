// The application-wide bindings the dispatcher in App.svelte compares against, and that the start
// screen lists: the user's saved choices with each missing one filled from its default.
import {
  DEFAULT_LOCAL_TERMINAL_HOTKEY,
  DEFAULT_SESSION_HOTKEYS,
  DEFAULT_TERMINAL_TOOL_HOTKEYS,
  type AppSettings,
} from '../api/settings';

export interface AppHotkeys {
  create: string;
  next: string;
  prev: string;
  close: string;
  localTerminal: string;
  search: string;
  multiInput: string;
  multiInputStop: string;
}

export const DEFAULT_APP_HOTKEYS: AppHotkeys = {
  ...DEFAULT_SESSION_HOTKEYS,
  localTerminal: DEFAULT_LOCAL_TERMINAL_HOTKEY,
  ...DEFAULT_TERMINAL_TOOL_HOTKEYS,
};

/** The bindings from saved settings. An empty field means "not set", never "no shortcut". */
export function hotkeysFromSettings(s: AppSettings): AppHotkeys {
  const d = DEFAULT_APP_HOTKEYS;
  return {
    create: s.sessionHotkeyCreate || d.create,
    next: s.sessionHotkeyNext || d.next,
    prev: s.sessionHotkeyPrev || d.prev,
    close: s.sessionHotkeyClose || d.close,
    localTerminal: s.localTerminalHotkey || d.localTerminal,
    search: s.terminalSearchHotkey || d.search,
    multiInput: s.multiInputHotkey || d.multiInput,
    multiInputStop: s.multiInputStopHotkey || d.multiInputStop,
  };
}
