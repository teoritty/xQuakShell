import type { Connection, Session } from '../../stores/appState';
import type { Team } from '../../backend/teamGateway';
import { describeVaultMove } from './vaultMovePrompt';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

// Echoes the key and its variables, so the test reads which sentence was chosen and what it names.
const t = (key: string, vars?: Record<string, string | number>) => `${key}${vars ? JSON.stringify(vars) : ''}`;
const conn = (id: string, vaultId?: string): Connection => ({ id, folderId: '', name: `n-${id}`, host: 'h', port: 22, order: 0, vaultId });
const teams: Team[] = [{ vault: 'v1', name: 'Ops', role: 'editor' }, { vault: 'v2', name: 'Web', role: 'owner' }];
const connections = [conn('p1'), conn('p2'), conn('t1', 'v1')];
const session = (connectionId: string, state: Session['state']) =>
  ({ sessionId: 's', connectionId, connectionName: '', state, errorMessage: '' }) as Session;

{
  const p = describeVaultMove({ connectionIds: ['p1'], vault: 'v1', folder: '' }, connections, teams, [], t);
  assert(p.title.includes('toTeamTitle') && p.title.includes('Ops'), `title = ${p.title}`);
  assert(p.message.includes('vaultMove.toTeam') && p.message.includes('n-p1'), `into a team names the team and the connection: ${p.message}`);
  assert(p.blocked === '', 'nothing blocks a connection without a session');
}
{
  const p = describeVaultMove({ connectionIds: ['t1'], vault: '', folder: '' }, connections, teams, [], t);
  assert(p.message.includes('toPersonal') && p.message.includes('Ops'), `out of a team names the team that loses it: ${p.message}`);
}
{
  const p = describeVaultMove({ connectionIds: ['t1'], vault: 'v2', folder: '' }, connections, teams, [], t);
  assert(p.message.includes('betweenTeams') && p.message.includes('"from":"Ops"') && p.message.includes('"team":"Web"'), `between teams: ${p.message}`);
}
{
  const p = describeVaultMove({ connectionIds: ['p1', 'p2'], vault: 'v1', folder: '' }, connections, teams, [], t);
  assert(p.message.includes('vaultMove.many') && p.message.includes('count\\":2'), `several are counted: ${p.message}`);
}
// An open session blocks the move - the moved connection gets a new id - and a closed one does not.
{
  const req = { connectionIds: ['p1'], vault: 'v1', folder: '' };
  assert(describeVaultMove(req, connections, teams, [session('p1', 'ready')], t).blocked.includes('sessionsOpen'), 'a live session blocks');
  assert(describeVaultMove(req, connections, teams, [session('p1', 'closed')], t).blocked === '', 'a closed session does not');
  assert(describeVaultMove(req, connections, teams, [session('p2', 'ready')], t).blocked === '', 'another connection session does not');
}

console.log('vaultMovePrompt: all assertions passed');
