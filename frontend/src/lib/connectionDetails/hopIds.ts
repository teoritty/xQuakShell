import type { JumpHop } from '../../stores/appState';

export const DRAFT_HOP_UI_PREFIX = 'draft-hop-';

/** True when hop id was generated only for UI list keys, not persisted by backend. */
export function isDraftHopUiId(id: string): boolean {
  return id.startsWith(DRAFT_HOP_UI_PREFIX) || id.startsWith('h-legacy-');
}

export function createDraftHopUiId(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return `${DRAFT_HOP_UI_PREFIX}${crypto.randomUUID()}`;
  }
  return `${DRAFT_HOP_UI_PREFIX}${Date.now()}`;
}

export function ensureHopUiId(hop: JumpHop, index: number): JumpHop {
  const withAuth = {
    ...hop,
    authMethod: hop.authMethod || 'key',
  };
  if (withAuth.id) return withAuth;
  return { ...withAuth, id: `h-legacy-${Date.now()}-${index}` };
}

/** Remove UI-only hop ids so backend remains the canonical ID source. */
export function stripDraftHopIdsForSave(hops: JumpHop[]): JumpHop[] {
  return hops.map((hop) => {
    if (!hop.id || isDraftHopUiId(hop.id)) {
      const { id: _id, ...rest } = hop;
      return { ...rest, id: '' } as JumpHop;
    }
    return hop;
  });
}

export function filterDraftHops(jumpHops: JumpHop[]): JumpHop[] {
  return jumpHops.filter((h) => h.host.trim() !== '');
}
