// What the person is asked before connections move between vaults. A move into a team shares the
// host with everyone in it, and a move out takes it away from all of them; both are decisions about
// other people, so the wording says who gains or loses the connection and what stays private.
import type { Connection, Session } from '../../stores/appState';
import type { Team } from '../../backend/teamGateway';
import type { VaultMoveRequest } from '../../stores/teamState';

type Translate = (key: string, vars?: Record<string, string | number>) => string;

export interface VaultMovePrompt {
  title: string;
  message: string;
  /** Non-empty when the move cannot go ahead yet, with the reason. */
  blocked: string;
}

/**
 * Builds the confirmation for req. An open session blocks the move: the moved connection gets a new
 * id, and the session - its reconnect, its tab's link back to the tree - would go on naming the old
 * one, which no longer exists.
 */
export function describeVaultMove(
  req: VaultMoveRequest,
  connections: Connection[],
  teams: Team[],
  sessions: Session[],
  t: Translate
): VaultMovePrompt {
  const moving = connections.filter((c) => req.connectionIds.includes(c.id));
  const what = moving.length === 1 ? t('security.tree.vaultMove.one', { name: moving[0].name }) : t('security.tree.vaultMove.many', { count: moving.length });
  const teamName = (vault: string) => teams.find((x) => x.vault === vault)?.name || t('sync.teams.unnamed');
  const sources = new Set(moving.map((c) => c.vaultId ?? ''));
  const from = sources.size === 1 ? [...sources][0] : '';
  const open = sessions.some((s) => req.connectionIds.includes(s.connectionId) && s.state !== 'closed');
  const blocked = open ? t('security.tree.vaultMove.sessionsOpen', { what }) : '';
  if (req.vault === '') {
    const team = teamName(from);
    return { title: t('security.tree.vaultMove.toPersonalTitle'), message: t('security.tree.vaultMove.toPersonal', { what, team }), blocked };
  }
  const team = teamName(req.vault);
  const message = from === ''
    ? t('security.tree.vaultMove.toTeam', { what, team })
    : t('security.tree.vaultMove.betweenTeams', { what, team, from: teamName(from) });
  return { title: t('security.tree.vaultMove.toTeamTitle', { team }), message, blocked };
}
