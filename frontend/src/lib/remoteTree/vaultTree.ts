import type { Connection, Folder } from '../../stores/appState';
import type { Team } from '../../backend/teamGateway';
import { buildTree, type DiscoveryTreeInput } from './buildTree';
import type { TreeNode, VaultGroup } from './types';

/**
 * Prefix on a team row's TreeNode.id. A C0 character, as for discovery rows: no connection or folder
 * id can contain it, so a team row never matches a selection helper that filters by id, and Delete
 * on a selection that includes one cannot reach anything real.
 */
export const VAULT_ID_PREFIX = 'vault\u0000';

export function vaultNodeId(vault: string): string {
  return VAULT_ID_PREFIX + vault;
}

export interface VaultTreeInput {
  folders: Folder[];
  connections: Connection[];
  expanded: Set<string>;
  query: string;
  discovery?: DiscoveryTreeInput;
  teams: Team[];
  /** Team vaults whose row the person folded. */
  collapsed: Set<string>;
}

/**
 * The tree with every synced vault in its place: the personal vault's items at the top level, as
 * they were before teams existed, then one row per team with that team's items beneath it. Without
 * teams the result is the personal tree alone, so a person who never turns sync on sees no change.
 *
 * Each vault is built from its own items only. A folder id is unique across vaults, but an item can
 * still name a folder that is not in its vault - one the other vault's member deleted, say - and
 * buildTree would then file it under a parent that is never drawn, which hides it. Such an item is
 * shown at the top level of its own vault instead.
 */
export function buildVaultTree(input: VaultTreeInput): TreeNode[] {
  const out = buildTree(...scoped(input, ''), input.expanded, input.query, input.discovery);
  for (const group of vaultGroups(input.teams, input.folders, input.connections)) {
    const node = groupNode(group, input);
    if (node) out.push(node);
  }
  return out;
}

/** The teams to draw: every listed team, then any vault this device holds items of but no longer lists. */
export function vaultGroups(teams: Team[], folders: Folder[], connections: Connection[]): VaultGroup[] {
  const out: VaultGroup[] = teams.map((t) => ({ vault: t.vault, name: t.name, writable: t.role !== 'viewer', viewer: t.role === 'viewer' }));
  const listed = new Set(out.map((g) => g.vault));
  for (const item of [...folders, ...connections]) {
    if (item.vaultId && !listed.has(item.vaultId)) {
      listed.add(item.vaultId);
      out.push({ vault: item.vaultId, name: '', writable: false, viewer: false });
    }
  }
  return out;
}

function scoped(input: VaultTreeInput, vault: string): [Folder[], Connection[]] {
  const folders = input.folders.filter((f) => (f.vaultId ?? '') === vault);
  const ids = new Set(folders.map((f) => f.id));
  const inVault = (id: string) => !id || ids.has(id);
  return [
    folders.map((f) => (inVault(f.parentId) ? f : { ...f, parentId: '' })),
    input.connections
      .filter((c) => (c.vaultId ?? '') === vault)
      .map((c) => (inVault(c.folderId) ? c : { ...c, folderId: '' })),
  ];
}

function groupNode(group: VaultGroup, input: VaultTreeInput): TreeNode | null {
  const children = shiftDepth(buildTree(...scoped(input, group.vault), input.expanded, input.query, input.discovery));
  const named = !!input.query && group.name.toLowerCase().includes(input.query.toLowerCase());
  if (input.query && children.length === 0 && !named) return null;
  const expanded = !input.collapsed.has(group.vault) || (!!input.query && children.length > 0);
  return {
    type: 'vault',
    id: vaultNodeId(group.vault),
    name: group.name,
    depth: 0,
    parentId: '',
    expanded,
    children: expanded ? children : [],
    group,
  };
}

function shiftDepth(nodes: TreeNode[]): TreeNode[] {
  return nodes.map((n) => ({ ...n, depth: n.depth + 1, children: n.children && shiftDepth(n.children) }));
}
