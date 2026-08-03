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
