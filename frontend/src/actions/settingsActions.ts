// Settings orchestration layer: thin wrappers over the atomic settings RPCs
// (fetchSettings / putSettings, in api/settings.ts) plus the appearance
// application helper. Moved verbatim from stores/api.ts. Neither
// fetchSettings nor putSettings needs an explicit missing-gateway guard here:
// both already route through callBackend, which performs its own
// getGateway() check and returns/throws its documented fallback before any
// backend call — so getSettings/saveSettings simply forward through.
import { fetchSettings, putSettings, type AppSettings } from '../api/settings';
import { applyUiScalePercent, DEFAULT_UI_SCALE_PERCENT } from '../lib/uiScale';
import { applyLocale, DEFAULT_LOCALE } from '../i18n/apply';

export async function getSettings(): Promise<AppSettings | null> {
  return fetchSettings();
}

export async function saveSettings(settings: Partial<AppSettings>): Promise<void> {
  return putSettings(settings);
}

export async function applyAppearanceSettings(): Promise<void> {
  const s = await getSettings();
  if (!s) return;
  applyUiScalePercent(s.uiScalePercent ?? DEFAULT_UI_SCALE_PERCENT);
  // The vault is the source of truth for the language. Startup drew the interface from the
  // localStorage mirror because the vault was still locked; this is where the two are reconciled,
  // and it also refreshes the mirror for the next launch.
  await applyLocale(s.language || DEFAULT_LOCALE);
}
