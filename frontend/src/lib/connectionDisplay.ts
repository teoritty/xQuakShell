import type { Connection } from '../stores/appState';

/** Returns the username shown in connection lists (default user, else first user). */
export function connectionDisplayUsername(conn: Connection): string {
  if (conn.defaultUserId && conn.users) {
    const match = conn.users.find((u) => u.id === conn.defaultUserId);
    if (match?.username) return match.username;
  }
  return conn.users?.[0]?.username ?? '';
}
