import type { Connection, Folder } from '../../stores/appState';
import type { Team } from '../../backend/teamGateway';
import { buildTree, flattenTree } from './buildTree';
import { buildVaultTree, vaultGroups, vaultNodeId, type VaultTreeInput } from './vaultTree';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const conn = (id: string, extra: Partial<Connection> = {}): Connection => ({ id, folderId: '', name: id, host: 'h', port: 22, order: 0, ...extra });
const folder = (id: string, extra: Partial<Folder> = {}): Folder => ({ id, name: id, parentId: '', order: 0, ...extra });
const ops: Team = { vault: 'v1', name: 'Ops', role: 'editor' };

function input(over: Partial<VaultTreeInput>): VaultTreeInput {
  return { folders: [], connections: [], expanded: new Set(), query: '', teams: [], collapsed: new Set(), ...over };
}

const shape = (nodes: ReturnType<typeof buildVaultTree>) =>
  flattenTree(nodes).map((n) => `${'  '.repeat(n.depth)}${n.type}:${n.name}`).join(' | ');

// Without teams it is the personal tree, unchanged: a person who never turns sync on sees no change.
{
  const folders = [folder('f')];
  const connections = [conn('a'), conn('b', { folderId: 'f' })];
  const expanded = new Set(['f']);
  const got = JSON.stringify(buildVaultTree(input({ folders, connections, expanded })));
  assert(got === JSON.stringify(buildTree(folders, connections, expanded, '')), 'with no teams the tree is the personal tree');
}

// Each team's items sit under the team, one level in; the personal ones stay where they were.
{
  const tree = buildVaultTree(input({
    teams: [ops],
    folders: [folder('tf', { vaultId: 'v1' })],
    connections: [conn('mine'), conn('shared', { vaultId: 'v1' }), conn('deep', { vaultId: 'v1', folderId: 'tf' })],
    expanded: new Set(['tf']),
  }));
  const want = 'connection:mine | vault:Ops |   folder:tf |     connection:deep |   connection:shared';
  assert(shape(tree) === want, `tree = ${shape(tree)}; want ${want}`);
  assert(tree[1].id === vaultNodeId('v1') && tree[1].group?.writable === true, 'the team row carries its vault and the role');
}

// A team with nothing in it yet still shows, so there is somewhere to add the first item.
assert(shape(buildVaultTree(input({ teams: [ops] }))) === 'vault:Ops', 'an empty team has its row');

// A folded team draws its row alone.
{
  const tree = buildVaultTree(input({ teams: [ops], connections: [conn('shared', { vaultId: 'v1' })], collapsed: new Set(['v1']) }));
  assert(shape(tree) === 'vault:Ops' && tree[0].expanded === false, `folded = ${shape(tree)}`);
}

// An item naming a folder its vault does not hold would be filed under a parent that is never drawn,
// which hid it. It shows at the top level of its own vault instead - personal ones included.
{
  const tree = buildVaultTree(input({
    teams: [ops],
    folders: [folder('pf')],
    connections: [conn('stray', { vaultId: 'v1', folderId: 'pf' }), conn('orphan', { folderId: 'gone' })],
  }));
  const want = 'folder:pf | connection:orphan | vault:Ops |   connection:stray';
  assert(shape(tree) === want, `tree = ${shape(tree)}; want ${want}`);
}

// A search opens a folded team that has a match, and drops a team without one.
{
  const web: Team = { vault: 'v2', name: 'Web', role: 'owner' };
  const tree = buildVaultTree(input({
    teams: [ops, web],
    connections: [conn('db-1', { vaultId: 'v1' }), conn('cache', { vaultId: 'v2' })],
    query: 'db',
    collapsed: new Set(['v1']),
  }));
  assert(shape(tree) === 'vault:Ops |   connection:db-1', `search = ${shape(tree)}`);
}

// The listed teams come first; a vault the device still holds items of but no longer lists - a team
// still loading, or one just left - is drawn too, as one nobody can add to.
{
  const viewer: Team = { vault: 'v2', name: 'Read', role: 'viewer' };
  const got = JSON.stringify(vaultGroups([ops, viewer], [], [conn('x', { vaultId: 'gone' })]));
  const want = JSON.stringify([
    { vault: 'v1', name: 'Ops', writable: true, viewer: false },
    { vault: 'v2', name: 'Read', writable: false, viewer: true },
    { vault: 'gone', name: '', writable: false, viewer: false },
  ]);
  assert(got === want, `groups = ${got}`);
}

console.log('vaultTree: all assertions passed');
