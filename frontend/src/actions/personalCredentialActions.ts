// A team connection's user whose credential each member supplies from their own vault: the host is
// shared, the credential is not. The editor shows and sets that member's link instead of the
// shared record's reference - a password typed there used to be written into the team's record,
// where it reached every member and authenticated none of them.
import { importPassword } from '../api/credentials';
import { bindCredentialRpc, listCredentialBindingsRpc, type CredentialBinding } from '../api/team';
import { showError, type Connection, type ConnectionUser } from '../stores/appState';
import { credentialBindings } from '../stores/syncState';
import { errorMessage } from './syncActions';

/** Whether userId of conn takes its credential from each member's own vault. */
export function linksPersonally(conn: Connection | null | undefined, userId: string): boolean {
  if (!conn?.vaultId) return false;
  return (conn.users ?? []).some((u) => u.id === userId && u.credentialSource === 'personal');
}

export async function refreshCredentialBindings(): Promise<void> {
  try {
    credentialBindings.set(await listCredentialBindingsRpc());
  } catch {
    // Without sync there are no team connections, and so nothing for the editor to overlay.
    credentialBindings.set([]);
  }
}

/**
 * Links this member's password or key to one user of a team connection. A password is stored in
 * the personal vault first; only its id goes into the link, which syncs to this member's devices.
 */
export async function linkPersonalCredential(
  conn: Connection,
  userId: string,
  credential: { password?: string; identity?: string },
): Promise<boolean> {
  try {
    const password = credential.password ? await importPassword(credential.password, `${conn.name} · ${userId}`) : '';
    if (credential.password && !password) return false;
    await bindCredentialRpc({ vault: conn.vaultId ?? '', connection: conn.id, user: userId, identity: credential.identity ?? '', password });
  } catch (e) {
    showError(errorMessage(e));
    return false;
  }
  await refreshCredentialBindings();
  return true;
}

/**
 * The users as this member sees them: a personally sourced user shows the method and credential of
 * this member's own link, or none while there is no link yet.
 */
export function withPersonalCredentials(conn: Connection | null | undefined, users: ConnectionUser[], links: CredentialBinding[]): ConnectionUser[] {
  if (!conn?.vaultId) return users;
  return users.map((u) => {
    if (u.credentialSource !== 'personal') return u;
    const link = links.find((b) => b.vault === conn.vaultId && b.connection === conn.id && b.user === u.id);
    if (link?.identity) return { ...u, authMethod: 'key', keyAuth: { identityIds: [link.identity] }, passAuth: undefined };
    if (link?.password) return { ...u, authMethod: 'password', passAuth: { passwordId: link.password }, keyAuth: undefined };
    return { ...u, keyAuth: undefined, passAuth: undefined };
  });
}
