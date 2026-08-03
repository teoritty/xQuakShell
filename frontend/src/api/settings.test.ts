import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { fetchSettings, putSettings, DEFAULT_SESSION_HOTKEYS, type AppSettings } from './settings';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  // fetchSettings normalizes hotkeys (via hotkeys module) and uiScalePercent
  const rawSettings = {
    uiScalePercent: 0,
    sessionHotkeyCreate: 'ctrl+shift+n',
    sessionHotkeyNext: '',
    sessionHotkeyPrev: 'ctrl+shift+tab',
    sessionHotkeyClose: 'ctrl+shift+q',
  } as AppSettings;
  fake.program('GetSettings', rawSettings);
  let result = await fetchSettings();
  assert(result !== null, 'fetchSettings returns settings');
  assert(result!.sessionHotkeyCreate === 'Ctrl+Shift+N', 'fetchSettings normalizes sessionHotkeyCreate');
  assert(result!.sessionHotkeyNext === DEFAULT_SESSION_HOTKEYS.next, 'fetchSettings defaults empty sessionHotkeyNext then normalizes');
  assert(result!.sessionHotkeyPrev === 'Ctrl+Shift+Tab', 'fetchSettings normalizes sessionHotkeyPrev');
  assert(result!.sessionHotkeyClose === 'Ctrl+Shift+Q', 'fetchSettings normalizes sessionHotkeyClose');
  assert(typeof result!.uiScalePercent === 'number', 'fetchSettings normalizes uiScalePercent');
  assert(get(lastError) === null, 'no error for successful fetchSettings');

  // fetchSettings: "vault is locked" is silenced (returns null, no lastError)
  fake = createFakeGateway();
  fake.program('GetSettings', () => { throw new Error('Vault Is Locked: cannot read settings'); });
  setGateway(fake);
  lastError.set(null);
  result = await fetchSettings();
  assert(result === null, 'fetchSettings returns null when vault is locked');
  assert(get(lastError) === null, 'fetchSettings does not report lastError for vault-locked case');

  // fetchSettings: other errors DO report via lastError and return null
  fake = createFakeGateway();
  fake.program('GetSettings', () => { throw new Error('boom'); });
  setGateway(fake);
  lastError.set(null);
  result = await fetchSettings();
  assert(result === null, 'fetchSettings falls back to null on unexpected error');
  assert(get(lastError) !== null, 'fetchSettings reports lastError for unexpected error');

  // putSettings normalizes the four hotkeys before SaveSettings
  fake = createFakeGateway();
  fake.program('SaveSettings', undefined);
  setGateway(fake);
  lastError.set(null);
  await putSettings({
    theme: 'dark',
    sessionHotkeyCreate: 'ctrl+n',
    sessionHotkeyNext: '',
    sessionHotkeyPrev: 'ctrl+shift+tab',
    sessionHotkeyClose: 'ctrl+shift+q',
  });
  const call = fake.calls.find((c) => c.method === 'SaveSettings');
  assert(!!call, 'SaveSettings called');
  const payload = call!.args[0] as AppSettings;
  assert(payload.theme === 'dark', 'putSettings passes through non-hotkey fields');
  assert(payload.sessionHotkeyCreate === 'Ctrl+N', 'putSettings normalizes sessionHotkeyCreate');
  assert(payload.sessionHotkeyNext === DEFAULT_SESSION_HOTKEYS.next, 'putSettings defaults+normalizes empty sessionHotkeyNext');
  assert(payload.sessionHotkeyPrev === 'Ctrl+Shift+Tab', 'putSettings normalizes sessionHotkeyPrev');
  assert(payload.sessionHotkeyClose === 'Ctrl+Shift+Q', 'putSettings normalizes sessionHotkeyClose');
  assert(get(lastError) === null, 'no error for successful putSettings');

  // putSettings failure reports via lastError and does not throw
  fake = createFakeGateway();
  fake.program('SaveSettings', () => { throw new Error('save failed'); });
  setGateway(fake);
  lastError.set(null);
  let threw: unknown = null;
  try {
    await putSettings({});
  } catch (e) {
    threw = e;
  }
  assert(threw === null, 'putSettings swallows errors (does not throw)');
  assert(get(lastError) !== null, 'putSettings failure reports error');

  // no gateway
  setGateway(null);
  lastError.set(null);
  result = await fetchSettings();
  assert(result === null, 'fetchSettings returns null with no gateway');
  await putSettings({});
  assert(get(lastError) === null, 'putSettings no-ops silently with no gateway');

  console.log('settings.test.ts passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
