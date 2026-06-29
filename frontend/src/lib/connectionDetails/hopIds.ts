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

/**
 * Adopt backend-assigned hop UUIDs without dropping in-progress draft rows.
 *
 * This is an intentional UI/persistence contract, not a defensive workaround:
 * empty-host hops are local-only editor rows and are deliberately excluded from
 * save payloads by filterDraftHops. When the backend returns only persisted hops,
 * the editor must merge canonical backend IDs back into the saved rows while
 * preserving unsaved local rows exactly as the user sees them. Rebuilding the
 * whole draft from the backend response would erase the user's in-progress form.
 *
 * Persisted hops are sent in filterDraftHops order, so saved hops zip 1:1 with
 * non-empty-host draft hops; empty-host rows are skipped and kept untouched.
 */
export function adoptPersistedHopIds(draftHops: JumpHop[], savedHops: JumpHop[]): JumpHop[] {
  let i = 0;
  return draftHops.map((hop) => {
    if (hop.host.trim() === '') return hop;
    const saved = savedHops[i++];
    return saved?.id ? { ...hop, id: saved.id } : hop;
  });
}
