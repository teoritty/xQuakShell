import { get } from 'svelte/store';
import type { Connection, Folder } from '../../stores/appState';
import { pendingVaultMove, teams } from '../../stores/teamState';
import { classifyVaultDrop, nodeVault, redirectAcrossVaults, refusesVaultDrop } from './vaultDnd';
import type { TreeNode } from './types';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const conn = (id: string, vaultId?: string): Connection => ({ id, folderId: '', name: id, host: 'h', port: 22, order: 0, vaultId });
const folder = (id: string, vaultId?: string): Folder => ({ id, name: id, parentId: '', order: 0, vaultId });
const connections = [conn('p1'), conn('p2'), conn('t1', 'v1'), conn('u1', 'v2')];
const folders = [folder('pf'), folder('tf', 'v1')];
const drag = (connectionIds: string[], folderIds: string[] = []) => ({ connectionIds, folderIds });

// Within one vault a drop is the ordinary folder change and reorder it always was.
assert(classifyVaultDrop(drag(['p1', 'p2'], ['pf']), '', connections, folders) === 'same', 'personal items into the personal vault');
assert(classifyVaultDrop(drag(['t1']), 'v1', connections, folders) === 'same', 'a team item within its team');

// Connections from elsewhere are a move - from either side, and from more than one team at once.
assert(classifyVaultDrop(drag(['p1']), 'v1', connections, folders) === 'move', 'personal into a team');
assert(classifyVaultDrop(drag(['t1']), '', connections, folders) === 'move', 'a team item out to the personal vault');
assert(classifyVaultDrop(drag(['t1', 'u1']), '', connections, folders) === 'move', 'from two teams to the personal vault');

// A folder never crosses, and a drag mixing items of the target with items from elsewhere has no
// single meaning: neither is done, so no indicator promises it.
assert(classifyVaultDrop(drag([], ['pf']), 'v1', connections, folders) === 'refuse', 'a personal folder into a team');
assert(classifyVaultDrop(drag(['p1'], ['tf']), 'v1', connections, folders) === 'refuse', 'a foreign connection with a folder of the target');
assert(classifyVaultDrop(drag(['p1', 't1']), 'v1', connections, folders) === 'refuse', 'a mixed drag');

// The vault of a drop target is the vault of what it is dropped on.
const node = (n: Partial<TreeNode>): TreeNode => ({ type: 'connection', id: 'x', name: 'x', depth: 0, parentId: '', ...n });
assert(nodeVault(node({ connection: conn('t1', 'v1') })) === 'v1', 'onto a team connection');
assert(nodeVault(node({ type: 'folder', folder: folder('pf') })) === '', 'onto a personal folder');
assert(nodeVault(node({ type: 'vault', group: { vault: 'v2', name: 'W', writable: true, viewer: false } })) === 'v2', 'onto a team row');

// A move is never done by the drop itself: it waits for the confirmation, with the target folder.
teams.set([{ vault: 'v1', name: 'Ops', role: 'editor' }, { vault: 'v2', name: 'Read', role: 'viewer' }]);
{
  pendingVaultMove.set(null);
  assert(redirectAcrossVaults(drag(['p1']), 'v1', 'tf', connections, folders), 'a cross-vault drop is handled here');
  const req = get(pendingVaultMove);
  assert(req?.vault === 'v1' && req.folder === 'tf' && req.connectionIds[0] === 'p1', `request = ${JSON.stringify(req)}`);
  pendingVaultMove.set(null);
  assert(redirectAcrossVaults(drag([], ['pf']), 'v1', '', connections, folders) && get(pendingVaultMove) === null, 'a refused drop asks nothing');
  assert(!redirectAcrossVaults(drag(['p1']), '', 'pf', connections, folders), 'a drop within the vault is left to the caller');
}

// A viewer moves nothing in or out: the backend would refuse it after the person confirmed, so the
// drop asks nothing and the indicator never promises it.
{
  pendingVaultMove.set(null);
  assert(redirectAcrossVaults(drag(['u1']), '', '', connections, folders) && get(pendingVaultMove) === null, "out of a viewer's team");
  assert(redirectAcrossVaults(drag(['p1']), 'v2', '', connections, folders) && get(pendingVaultMove) === null, "into a viewer's team");
  const onto = node({ type: 'vault', group: { vault: 'v2', name: 'Read', writable: false, viewer: true } });
  assert(refusesVaultDrop(drag(['p1']), onto, connections, folders), 'no indicator over a team the account only views');
  assert(!refusesVaultDrop(drag(['p1']), node({ connection: conn('t1', 'v1') }), connections, folders), 'an editor team takes the drop');
}

console.log('vaultDnd: all assertions passed');
