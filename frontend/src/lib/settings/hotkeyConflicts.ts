import { normalizeHotkey } from '../../hotkeys/hotkeys';

export interface SessionHotkeyDraft {
  sessionHotkeyCreate: string;
  sessionHotkeyNext: string;
  sessionHotkeyPrev: string;
  sessionHotkeyClose: string;
}

/**
 * Reports the first pair of session hotkeys bound to the same combination, as a message to show
 * beside the fields.
 *
 * Comparison is on the normalized form, not the typed text: "Ctrl+Shift+T" and "ctrl+shift+t" are
 * the same binding, and a check on the raw strings would let a user save two shortcuts that fight
 * over every keypress. An empty field is not a conflict — it means that action has no shortcut.
 */
export function findHotkeyConflict(hotkeys: SessionHotkeyDraft): string {
  const entries = [
    { label: 'Create session', value: normalizeHotkey(hotkeys.sessionHotkeyCreate) },
    { label: 'Next session', value: normalizeHotkey(hotkeys.sessionHotkeyNext) },
    { label: 'Previous session', value: normalizeHotkey(hotkeys.sessionHotkeyPrev) },
    { label: 'Close session', value: normalizeHotkey(hotkeys.sessionHotkeyClose) },
  ];
  for (let i = 0; i < entries.length; i++) {
    for (let j = i + 1; j < entries.length; j++) {
      if (entries[i].value && entries[i].value === entries[j].value) {
        return `${entries[i].label} conflicts with ${entries[j].label}`;
      }
    }
  }
  return '';
}
