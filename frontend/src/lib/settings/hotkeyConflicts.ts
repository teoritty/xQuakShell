import { normalizeHotkey } from '../../hotkeys/hotkeys';

export interface SessionHotkeyDraft {
  sessionHotkeyCreate: string;
  sessionHotkeyNext: string;
  sessionHotkeyPrev: string;
  sessionHotkeyClose: string;
}

/** Resolves a message key to its text. The dialog passes `$t`; tests pass whatever they need. */
export type ConflictLabels = (key: string, vars?: Record<string, string | number>) => string;

/**
 * Reports the first pair of session hotkeys bound to the same combination, as a message to show
 * beside the fields.
 *
 * Comparison is on the normalized form, not the typed text: "Ctrl+Shift+T" and "ctrl+shift+t" are
 * the same binding, and a check on the raw strings would let a user save two shortcuts that fight
 * over every keypress. An empty field is not a conflict — it means that action has no shortcut.
 *
 * The message is assembled through the caller's lookup rather than from literals so it follows the
 * interface language; the two action names are substituted rather than concatenated, because a
 * translation is free to put them in the other order.
 */
export function findHotkeyConflict(hotkeys: SessionHotkeyDraft, label: ConflictLabels): string {
  const entries = [
    { key: 'settings.hotkeys.action.create', value: normalizeHotkey(hotkeys.sessionHotkeyCreate) },
    { key: 'settings.hotkeys.action.next', value: normalizeHotkey(hotkeys.sessionHotkeyNext) },
    { key: 'settings.hotkeys.action.prev', value: normalizeHotkey(hotkeys.sessionHotkeyPrev) },
    { key: 'settings.hotkeys.action.close', value: normalizeHotkey(hotkeys.sessionHotkeyClose) },
  ];
  for (let i = 0; i < entries.length; i++) {
    for (let j = i + 1; j < entries.length; j++) {
      if (entries[i].value && entries[i].value === entries[j].value) {
        return label('settings.hotkeys.conflict', {
          first: label(entries[i].key),
          second: label(entries[j].key),
        });
      }
    }
  }
  return '';
}
