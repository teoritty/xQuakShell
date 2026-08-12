import type { ConnectionUser, JumpHop } from '../../stores/appState';

export function addIdentityToUser(
  users: ConnectionUser[],
  userId: string,
  keyId: string,
): ConnectionUser[] {
  return users.map((u) => {
    if (u.id !== userId) return u;
    const ids = u.keyAuth?.identityIds || [];
    return { ...u, keyAuth: { identityIds: [...ids, keyId] } };
  });
}

export function removeIdentityFromUser(
  users: ConnectionUser[],
  userId: string,
  keyId: string,
): ConnectionUser[] {
  return users.map((u) => {
    if (u.id !== userId) return u;
    const ids = (u.keyAuth?.identityIds || []).filter((i) => i !== keyId);
    return { ...u, keyAuth: { identityIds: ids } };
  });
}

export function setUserPassword(
  users: ConnectionUser[],
  userId: string,
  passwordId: string,
): ConnectionUser[] {
  return users.map((u) =>
    u.id === userId ? { ...u, passAuth: { passwordId } } : u,
  );
}

export function addIdentityToHop(
  hops: JumpHop[],
  hopId: string,
  keyId: string,
): JumpHop[] {
  return hops.map((h) => {
    if (h.id !== hopId) return h;
    const ids = h.keyAuth?.identityIds || [];
    return { ...h, keyAuth: { identityIds: [...ids, keyId] } };
  });
}

export function removeIdentityFromHop(
  hops: JumpHop[],
  hopId: string,
  keyId: string,
): JumpHop[] {
  return hops.map((h) => {
    if (h.id !== hopId) return h;
    const ids = (h.keyAuth?.identityIds || []).filter((i) => i !== keyId);
    return { ...h, keyAuth: { identityIds: ids } };
  });
}

export function setHopPassword(
  hops: JumpHop[],
  hopId: string,
  passwordId: string,
): JumpHop[] {
  return hops.map((h) =>
    h.id === hopId ? { ...h, passAuth: { passwordId } } : h,
  );
}

/** A connection user or a jump hop — the two things a key can be attached to. */
export interface KeyTargets {
  users: ConnectionUser[];
  hops: JumpHop[];
}

/**
 * Adds a key to whichever target carries the given id.
 *
 * The caller holds one picker for both users and hops and knows only the id it opened it for, so
 * the decision of which list that id belongs to lives here rather than being repeated at the two
 * call sites that would otherwise have to make it.
 */
export function addIdentityToTarget(targets: KeyTargets, targetId: string, keyId: string): KeyTargets {
  if (targets.users.some((u) => u.id === targetId)) {
    return { users: addIdentityToUser(targets.users, targetId, keyId), hops: targets.hops };
  }
  if (targets.hops.some((h) => h.id === targetId)) {
    return { users: targets.users, hops: addIdentityToHop(targets.hops, targetId, keyId) };
  }
  return targets;
}

/** The keys a target already uses, so a picker can show them as taken rather than offer them twice. */
export function identitiesForTarget(targets: KeyTargets, targetId: string): string[] {
  const user = targets.users.find((u) => u.id === targetId);
  if (user) return user.keyAuth?.identityIds || [];
  return targets.hops.find((h) => h.id === targetId)?.keyAuth?.identityIds || [];
}
