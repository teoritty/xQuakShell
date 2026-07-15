// Atomic settings RPC wrappers. `fetchSettings`/`putSettings` are thin
// wrappers around single backend RPC calls, routed through callBackend for
// uniform error handling, but the hotkey/uiScale normalization on both the
// read and write sides is pure input/output shaping and stays inside these
// functions (not lifted to an orchestration layer). No store access here.
import { callBackend, callBackendVoid } from '../backend/callBackend';
import { normalizeHotkey } from '../hotkeys/hotkeys';
import { normalizeUiScalePercent } from '../lib/uiScale';

export interface SessionHotkeysSettings {
  create: string;
  next: string;
  prev: string;
  close: string;
}

export interface AppSettings {
  lockoutEnabled: boolean;
  lockoutIdleMinutes: number;
  lockOnMinimize: boolean;
  terminalFontFamily: string;
  terminalFontSize: number;
  terminalFontColor: string;
  theme: string;
  uiScalePercent: number;
  pingEnabled: boolean;
  pingMode: string;
  pingIntervalSeconds: number;
  pingIntervalMin: number;
  maxConcurrentPings: number;
  externalEditorPath: string;
  transferSpeedLimitKbps: number;
  connectionTimeoutSeconds: number;
  maxConcurrentTransfers: number;
  sessionHotkeyCreate: string;
  sessionHotkeyNext: string;
  sessionHotkeyPrev: string;
  sessionHotkeyClose: string;
  auditLogEnabled: boolean;
  auditRetentionMode: string;
  auditRetentionDays: number;
  auditRetentionCount: number;
  auditShowUsername: boolean;
  auditShowConnection: boolean;
  debugLogWindowEnabled: boolean;
}

export interface AuditEntry {
  id: number;
  timestamp: string;
  category: string;
  sessionId: string;
  connectionId: string;
  connectionName: string;
  host: string;
  username: string;
  input: string;
  redacted: boolean;
}

export interface AuditSessionState {
  logSecretsEnabled: boolean;
}

export const DEFAULT_SESSION_HOTKEYS: SessionHotkeysSettings = {
  create: 'Ctrl+Shift+N',
  next: 'Ctrl+Tab',
  prev: 'Ctrl+Shift+Tab',
  close: 'Ctrl+Shift+Q',
};

// `fetchSettings` preserves the original "vault is locked" silence exactly:
// that error is expected during startup before unlock, so it must return
// `null` without reporting via lastError (callBackend's `silence` predicate
// matches the original's case-insensitive substring check).
export async function fetchSettings(): Promise<AppSettings | null> {
  return callBackend(
    'Get settings',
    null,
    async (app) => {
      const s: AppSettings = await app.GetSettings();
      s.sessionHotkeyCreate = normalizeHotkey(s.sessionHotkeyCreate || DEFAULT_SESSION_HOTKEYS.create);
      s.sessionHotkeyNext = normalizeHotkey(s.sessionHotkeyNext || DEFAULT_SESSION_HOTKEYS.next);
      s.sessionHotkeyPrev = normalizeHotkey(s.sessionHotkeyPrev || DEFAULT_SESSION_HOTKEYS.prev);
      s.sessionHotkeyClose = normalizeHotkey(s.sessionHotkeyClose || DEFAULT_SESSION_HOTKEYS.close);
      s.uiScalePercent = normalizeUiScalePercent(s.uiScalePercent);
      return s;
    },
    { silence: (msg) => msg.toLowerCase().includes('vault is locked') },
  );
}

// VersionInfo carries the three distinct versions shown in the About panel (ADR-012):
// the application release, the plugin core version, and the frozen plugin API envelope.
export interface VersionInfo {
  appVersion: string;
  coreVersion: string;
  pluginApiVersion: string;
}

// fetchVersionInfo returns the app/core/pluginApi versions, or null if the backend is
// unavailable (the About panel falls back to placeholders in that case).
export async function fetchVersionInfo(): Promise<VersionInfo | null> {
  return callBackend(
    'Get version info',
    null,
    async (app) => {
      if (!app.GetVersionInfo) return null;
      return (await app.GetVersionInfo()) as VersionInfo;
    },
  );
}

export async function putSettings(settings: Partial<AppSettings>): Promise<void> {
  return callBackendVoid('Save settings', (app) => {
    const payload = {
      ...settings,
      sessionHotkeyCreate: normalizeHotkey(settings.sessionHotkeyCreate || DEFAULT_SESSION_HOTKEYS.create),
      sessionHotkeyNext: normalizeHotkey(settings.sessionHotkeyNext || DEFAULT_SESSION_HOTKEYS.next),
      sessionHotkeyPrev: normalizeHotkey(settings.sessionHotkeyPrev || DEFAULT_SESSION_HOTKEYS.prev),
      sessionHotkeyClose: normalizeHotkey(settings.sessionHotkeyClose || DEFAULT_SESSION_HOTKEYS.close),
    };
    return app.SaveSettings(payload as AppSettings);
  });
}
