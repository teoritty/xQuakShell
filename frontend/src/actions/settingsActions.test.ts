import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { getSettings, saveSettings, applyAppearanceSettings } from './settingsActions';
import { lastError } from '../stores/appState';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function reset() {
  lastError.set(null);
}

async function run() {
  // --- getSettings -----------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('GetSettings', {
      uiScalePercent: 120,
      sessionHotkeyCreate: '',
      sessionHotkeyNext: '',
      sessionHotkeyPrev: '',
      sessionHotkeyClose: '',
    });
    setGateway(fake);

    const s = await getSettings();
    assert(s !== null && s.uiScalePercent === 120, 'getSettings returns the settings object from GetSettings');
  }

  // getSettings: missing gateway silently returns null (vault-is-locked style silence
  // is handled inside fetchSettings/callBackend; here we exercise a fully absent gateway).
  {
    reset();
    setGateway(null);
    const s = await getSettings();
    assert(s === null, 'getSettings returns null when the gateway is absent');
  }

  // --- saveSettings ------------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('SaveSettings', undefined);
    setGateway(fake);
    await saveSettings({ uiScalePercent: 150 });
    const call = fake.calls.find((c) => c.method === 'SaveSettings');
    assert((call?.args[0] as { uiScalePercent: number }).uiScalePercent === 150, 'saveSettings forwards the settings payload to SaveSettings');
  }

  // --- applyAppearanceSettings --------------------------------------------

  // When getSettings() resolves to null (e.g. absent gateway), applyAppearanceSettings
  // short-circuits before touching `document` (unavailable in this Node test env).
  {
    reset();
    setGateway(null);
    let threw: unknown = null;
    try {
      await applyAppearanceSettings();
    } catch (e) {
      threw = e;
    }
    assert(threw === null, 'applyAppearanceSettings does not throw when getSettings resolves to null');
  }

  console.log('settingsActions.test passed');
}

run();
