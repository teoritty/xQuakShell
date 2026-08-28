import { DEFAULT_LOCAL_TERMINAL_HOTKEY, DEFAULT_SESSION_HOTKEYS, type AppSettings } from '../../api/settings';
import { normalizeHotkey } from '../../hotkeys/hotkeys';
import { DEFAULT_UI_SCALE_PERCENT } from '../uiScale';

/**
 * The settings the dialog edits, as one record.
 *
 * It exists so the defaults live in one testable place. They used to be thirty `?? 5` fallbacks
 * spread through the dialog's load function, where the value a field falls back to was visible only
 * to whoever was reading that line, and adding a setting meant remembering to touch three places in
 * the same component.
 *
 * Session state the dialog shows but does not save - whether secret logging is on for this run, the
 * interface scale the dialog opened with - is deliberately not here. This is the payload, and
 * nothing else.
 */
export interface SettingsDraft {
  lockoutEnabled: boolean;
  lockoutIdleMinutes: number;
  lockOnMinimize: boolean;
  terminalFontFamily: string;
  terminalFontSize: number;
  terminalFontColor: string;
  externalEditorPath: string;
  theme: string;
  language: string;
  uiScalePercent: number;
  pingEnabled: boolean;
  pingMode: string;
  pingIntervalSeconds: number;
  maxConcurrentPings: number;
  transferSpeedLimitKbps: number;
  connectionTimeoutSeconds: number;
  maxConcurrentTransfers: number;
  defaultUploadExistsAction: string;
  defaultDownloadExistsAction: string;
  sessionHotkeyCreate: string;
  sessionHotkeyNext: string;
  sessionHotkeyPrev: string;
  sessionHotkeyClose: string;
  localTerminalShellId: string;
  localTerminalHotkey: string;
  auditLogEnabled: boolean;
  auditRetentionMode: string;
  auditRetentionDays: number;
  auditRetentionCount: number;
  auditShowUsername: boolean;
  auditShowConnection: boolean;
  debugLogWindowEnabled: boolean;
  debugLogLevel: string;
  updateCheckOnStartup: boolean;
}

export const DEFAULT_TERMINAL_FONT = 'Cascadia Code, Consolas, Courier New, monospace';

/** The values shown before anything is loaded, and the fallback for every field the backend omits. */
export function defaultSettingsDraft(): SettingsDraft {
  return {
    lockoutEnabled: false,
    lockoutIdleMinutes: 5,
    lockOnMinimize: false,
    terminalFontFamily: DEFAULT_TERMINAL_FONT,
    terminalFontSize: 14,
    terminalFontColor: '#cccccc',
    externalEditorPath: '',
    theme: 'dark',
    language: 'en',
    uiScalePercent: DEFAULT_UI_SCALE_PERCENT,
    pingEnabled: true,
    pingMode: 'interval',
    pingIntervalSeconds: 5,
    maxConcurrentPings: 16,
    transferSpeedLimitKbps: 0,
    connectionTimeoutSeconds: 15,
    maxConcurrentTransfers: 4,
    defaultUploadExistsAction: 'ask',
    defaultDownloadExistsAction: 'ask',
    sessionHotkeyCreate: DEFAULT_SESSION_HOTKEYS.create,
    sessionHotkeyNext: DEFAULT_SESSION_HOTKEYS.next,
    sessionHotkeyPrev: DEFAULT_SESSION_HOTKEYS.prev,
    sessionHotkeyClose: DEFAULT_SESSION_HOTKEYS.close,
    localTerminalShellId: '',
    localTerminalHotkey: DEFAULT_LOCAL_TERMINAL_HOTKEY,
    auditLogEnabled: false,
    auditRetentionMode: 'days',
    auditRetentionDays: 30,
    auditRetentionCount: 100,
    auditShowUsername: false,
    auditShowConnection: false,
    debugLogWindowEnabled: false,
    // Mirrors loghub.DefaultLevel. The backend stores this field with `omitempty`, so an install
    // that never touched Developer settings sends nothing and the host resolves its own default;
    // if the two disagree the dialog reports a level the process is not running at.
    debugLogLevel: 'warn',
    updateCheckOnStartup: true,
  };
}

/**
 * Builds the editable draft from what the backend returned.
 *
 * A null settings object means the vault is locked or the read failed, and the answer there is the
 * defaults rather than a half-filled form: showing a blank font size next to a filled timeout would
 * invite the user to save the blank one.
 *
 * Numbers and booleans fall back on `??` so a legitimate 0 or false survives; strings fall back on
 * `||` so an empty string is treated as unset, which is what an empty text field means here.
 */
export function draftFromSettings(s: AppSettings | null): SettingsDraft {
  const d = defaultSettingsDraft();
  if (!s) return d;
  return {
    lockoutEnabled: s.lockoutEnabled ?? d.lockoutEnabled,
    lockoutIdleMinutes: s.lockoutIdleMinutes ?? d.lockoutIdleMinutes,
    lockOnMinimize: s.lockOnMinimize ?? d.lockOnMinimize,
    terminalFontFamily: s.terminalFontFamily || d.terminalFontFamily,
    terminalFontSize: s.terminalFontSize || d.terminalFontSize,
    terminalFontColor: s.terminalFontColor || d.terminalFontColor,
    externalEditorPath: s.externalEditorPath || d.externalEditorPath,
    theme: s.theme || d.theme,
    language: s.language || d.language,
    uiScalePercent: s.uiScalePercent ?? d.uiScalePercent,
    pingEnabled: s.pingEnabled ?? d.pingEnabled,
    pingMode: s.pingMode ?? d.pingMode,
    pingIntervalSeconds: s.pingIntervalSeconds ?? d.pingIntervalSeconds,
    maxConcurrentPings: s.maxConcurrentPings ?? d.maxConcurrentPings,
    transferSpeedLimitKbps: s.transferSpeedLimitKbps ?? d.transferSpeedLimitKbps,
    connectionTimeoutSeconds: s.connectionTimeoutSeconds ?? d.connectionTimeoutSeconds,
    maxConcurrentTransfers: s.maxConcurrentTransfers ?? d.maxConcurrentTransfers,
    defaultUploadExistsAction: s.defaultUploadExistsAction || d.defaultUploadExistsAction,
    defaultDownloadExistsAction: s.defaultDownloadExistsAction || d.defaultDownloadExistsAction,
    sessionHotkeyCreate: normalizeHotkey(s.sessionHotkeyCreate || d.sessionHotkeyCreate),
    sessionHotkeyNext: normalizeHotkey(s.sessionHotkeyNext || d.sessionHotkeyNext),
    sessionHotkeyPrev: normalizeHotkey(s.sessionHotkeyPrev || d.sessionHotkeyPrev),
    sessionHotkeyClose: normalizeHotkey(s.sessionHotkeyClose || d.sessionHotkeyClose),
    // An empty shell id is meaningful - it means this platform's default - so it is carried
    // through rather than filled in from the default draft.
    localTerminalShellId: s.localTerminalShellId ?? d.localTerminalShellId,
    localTerminalHotkey: normalizeHotkey(s.localTerminalHotkey || d.localTerminalHotkey),
    auditLogEnabled: s.auditLogEnabled ?? d.auditLogEnabled,
    auditRetentionMode: s.auditRetentionMode ?? d.auditRetentionMode,
    auditRetentionDays: s.auditRetentionDays ?? d.auditRetentionDays,
    auditRetentionCount: s.auditRetentionCount ?? d.auditRetentionCount,
    auditShowUsername: s.auditShowUsername ?? d.auditShowUsername,
    auditShowConnection: s.auditShowConnection ?? d.auditShowConnection,
    debugLogWindowEnabled: s.debugLogWindowEnabled ?? d.debugLogWindowEnabled,
    debugLogLevel: s.debugLogLevel || d.debugLogLevel,
    updateCheckOnStartup: s.updateCheckOnStartup ?? d.updateCheckOnStartup,
  };
}

/**
 * Turns the draft into the payload to save.
 *
 * The hotkeys are normalized on the way out as well as on the way in, because the field records
 * whatever the user's last keystroke produced and the stored form has to be the one the dispatcher
 * compares against.
 */
export function draftToSettings(d: SettingsDraft): Partial<AppSettings> {
  return {
    ...d,
    sessionHotkeyCreate: normalizeHotkey(d.sessionHotkeyCreate),
    sessionHotkeyNext: normalizeHotkey(d.sessionHotkeyNext),
    sessionHotkeyPrev: normalizeHotkey(d.sessionHotkeyPrev),
    sessionHotkeyClose: normalizeHotkey(d.sessionHotkeyClose),
    localTerminalHotkey: normalizeHotkey(d.localTerminalHotkey),
  };
}
