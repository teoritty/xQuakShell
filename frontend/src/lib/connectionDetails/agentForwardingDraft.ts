import type { AgentForwarding, SSHIdentityMeta } from '../../stores/appState';

/** The backend's bounds (ADR-022): a window of 1 to 60 minutes, 15 unless the user picks another. */
export const DEFAULT_WINDOW_MINUTES = 15;
export const MAX_WINDOW_MINUTES = 60;

/** Settings for a connection that has never had forwarding: off, no keys, ask every time. */
export function emptyAgentForwarding(): AgentForwarding {
  return { enabled: false, identityIds: [], confirm: 'each', windowMinutes: DEFAULT_WINDOW_MINUTES };
}

export function setForwardingEnabled(af: AgentForwarding | null, enabled: boolean): AgentForwarding {
  return { ...(af ?? emptyAgentForwarding()), enabled };
}

/** Adds the key if absent, removes it if present. */
export function toggleForwardedKey(af: AgentForwarding | null, identityId: string): AgentForwarding {
  const base = af ?? emptyAgentForwarding();
  const ids = base.identityIds.includes(identityId)
    ? base.identityIds.filter((id) => id !== identityId)
    : [...base.identityIds, identityId];
  return { ...base, identityIds: ids };
}

export function setConfirmPolicy(af: AgentForwarding | null, confirm: AgentForwarding['confirm']): AgentForwarding {
  return { ...(af ?? emptyAgentForwarding()), confirm };
}

/** Clamps to the backend's range, so the editor never saves a window the backend would refuse. */
export function setWindowMinutes(af: AgentForwarding | null, minutes: number): AgentForwarding {
  const whole = Number.isFinite(minutes) ? Math.round(minutes) : DEFAULT_WINDOW_MINUTES;
  return { ...(af ?? emptyAgentForwarding()), windowMinutes: Math.min(MAX_WINDOW_MINUTES, Math.max(1, whole)) };
}

/** Keys the user has allowed to be forwarded - the only ones the editor offers. */
export function forwardableKeys(identities: SSHIdentityMeta[]): SSHIdentityMeta[] {
  return identities.filter((i) => i.allowForwarding === true);
}

/**
 * Keys this connection lists whose own consent has since been withdrawn. The backend ignores them,
 * and the editor names them so the user is not left wondering why a listed key is never offered.
 */
export function listedWithoutConsent(af: AgentForwarding | null, identities: SSHIdentityMeta[]): SSHIdentityMeta[] {
  if (!af) return [];
  return identities.filter((i) => af.identityIds.includes(i.id) && i.allowForwarding !== true);
}
