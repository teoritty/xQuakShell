import assert from 'node:assert/strict';
import { defaultSettingsDraft, draftFromSettings, draftToSettings } from './settingsDraft';
import { findHotkeyConflict } from './hotkeyConflicts';
import type { AppSettings } from '../../api/settings';

function settings(overrides: Partial<AppSettings>): AppSettings {
  return { ...(draftToSettings(defaultSettingsDraft()) as AppSettings), ...overrides };
}

// A locked vault reads as null, and a half-filled form there would invite the user to save the
// blanks over settings they never saw.
{
  assert.deepEqual(
    draftFromSettings(null),
    defaultSettingsDraft(),
    'a null read must produce the defaults, not a partially populated draft',
  );
}

// `??` and `||` are not interchangeable here: a real 0 or false has to survive, while an empty
// string means "unset" for a text field.
{
  const d = draftFromSettings(settings({
    transferSpeedLimitKbps: 0,
    pingEnabled: false,
    updateCheckOnStartup: false,
    auditRetentionDays: 0,
  }));
  assert.equal(d.transferSpeedLimitKbps, 0, 'a speed limit of 0 means unlimited and must not fall back');
  assert.equal(d.pingEnabled, false, 'ping disabled must not fall back to the default true');
  assert.equal(d.updateCheckOnStartup, false, 'update check disabled must not fall back to true');
  assert.equal(d.auditRetentionDays, 0, 'a retention of 0 days must survive as the value it is');
}

{
  const d = draftFromSettings(settings({ terminalFontFamily: '', theme: '', language: '' }));
  const defaults = defaultSettingsDraft();
  assert.equal(d.terminalFontFamily, defaults.terminalFontFamily, 'an empty font stack falls back');
  assert.equal(d.theme, defaults.theme, 'an empty theme falls back');
  assert.equal(d.language, defaults.language, 'an empty language falls back to English');
}

// The stored form has to be the one the dispatcher compares against, whatever the field last held.
{
  const d = defaultSettingsDraft();
  d.sessionHotkeyCreate = 'CTRL+SHIFT+t';
  const saved = draftToSettings(d);
  assert.equal(
    saved.sessionHotkeyCreate,
    draftFromSettings(settings({ sessionHotkeyCreate: 'ctrl+shift+T' })).sessionHotkeyCreate,
    'the same binding typed in different cases must save identically',
  );
}

{
  // The message is assembled from the pack, so the test supplies the lookup rather than a copy of
  // the English wording - a reworded translation must not fail this.
  const label = (key: string, vars?: Record<string, string | number>) =>
    vars ? `${vars.first}|${vars.second}` : key;

  const d = defaultSettingsDraft();
  assert.equal(findHotkeyConflict(d, label), '', 'the shipped defaults must not conflict with each other');

  d.sessionHotkeyNext = d.sessionHotkeyCreate;
  assert.equal(
    findHotkeyConflict(d, label),
    'settings.hotkeys.action.create|settings.hotkeys.action.next',
    'two actions on one binding must be reported by both names, in binding order',
  );
}

// An unbound action is not a conflict, however many of them there are.
{
  const d = defaultSettingsDraft();
  d.sessionHotkeyNext = '';
  d.sessionHotkeyPrev = '';
  assert.equal(
    findHotkeyConflict(d, (key) => key),
    '',
    'empty fields mean "no shortcut", not "the same shortcut"',
  );
}

console.log('settingsDraft.test passed');
