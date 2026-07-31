import type { Connection, Folder } from '../../stores/appState';
import type { DiscoverySnapshot } from '../../api/discovery';
import { buildDiscoverySubtree } from './discoveryTree';
import type { TreeNode } from './types';

/**
 * Discovery state, passed in whole rather than reached for. buildTree stays a
 * pure function of its arguments, and the subtree itself is built by
 * discoveryTree.ts — this module knows only where to splice the result in.
 */
export interface DiscoveryTreeInput {
  snapshots: Map<string, DiscoverySnapshot>;
  /** connectionId -> expanded discoveryKey set; '' present means the root is open. */
  expanded: Map<string, Set<string>>;
}

export function matchesSearch(q: string, name: string, host?: string): boolean {
  const lower = q.toLowerCase();
  if (name.toLowerCase().includes(lower)) return true;
  if (host && host.toLowerCase().includes(lower)) return true;
  return false;
}

export function buildTree(
  folderList: Folder[],
  connList: Connection[],
  expanded: Set<string>,
  query: string,
  discovery?: DiscoveryTreeInput
): TreeNode[] {
  const folderMap = new Map<string, Folder[]>();
  const connMap = new Map<string, Connection[]>();

  for (const f of folderList) {
    const pid = f.parentId || '';
    if (!folderMap.has(pid)) folderMap.set(pid, []);
    folderMap.get(pid)!.push(f);
  }

  for (const c of connList) {
    const fid = c.folderId || '';
    if (!connMap.has(fid)) connMap.set(fid, []);
    connMap.get(fid)!.push(c);
  }

  function hasMatchInSubtree(parentId: string): boolean {
    for (const c of connMap.get(parentId) || []) {
      if (matchesSearch(query, c.name, c.host)) return true;
    }
    for (const f of folderMap.get(parentId) || []) {
      if (matchesSearch(query, f.name)) return true;
      if (hasMatchInSubtree(f.id)) return true;
    }
    return false;
  }

  function buildLevel(parentId: string, depth: number): TreeNode[] {
    const nodes: TreeNode[] = [];
    const subFolders = (folderMap.get(parentId) || []).sort((a, b) => a.order - b.order);
    for (const f of subFolders) {
      const childMatches = hasMatchInSubtree(f.id);
      const selfMatches = matchesSearch(query, f.name);
      const showFolder = !query || selfMatches || childMatches;
      if (!showFolder) continue;

      const isExpanded = expanded.has(f.id) || (query && childMatches);
      const node: TreeNode = {
        type: 'folder',
        id: f.id,
        name: f.name,
        depth,
        parentId,
        folder: f,
        expanded: isExpanded,
        children: isExpanded ? buildLevel(f.id, depth + 1) : [],
      };
      nodes.push(node);
    }
    const subConns = (connMap.get(parentId) || []).sort((a, b) => a.order - b.order);
    for (const c of subConns) {
      if (query && !matchesSearch(query, c.name, c.host)) continue;
      const connNode: TreeNode = {
        type: 'connection',
        id: c.id,
        name: c.name,
        depth,
        parentId,
        connection: c,
        tags: c.tags || [],
      };
      // The one point where discovery enters the connection tree. A search query
      // deliberately does NOT auto-expand a subtree: expanding is what publishes
      // an `observe`, so a single keystroke would fan out observe/publish across
      // every connection at once. Search filters the rows already loaded; the
      // UI says so rather than pretending the subtree was searched.
      const discoveryExpandedKeys = discovery?.expanded.get(c.id);
      if (discoveryExpandedKeys?.has('')) {
        connNode.expanded = true;
        connNode.children = buildDiscoverySubtree({
          connectionId: c.id,
          snapshot: discovery?.snapshots.get(c.id),
          expandedKeys: discoveryExpandedKeys,
          baseDepth: depth,
          parentId: c.id,
        });
      }
      nodes.push(connNode);
    }
    return nodes;
  }

  return buildLevel('', 0);
}

export function flattenTree(nodes: TreeNode[]): TreeNode[] {
  const result: TreeNode[] = [];
  for (const n of nodes) {
    result.push(n);
    // Folders nest folders; connections nest their discovery subtree, which
    // arrives already flat, so the recursion bottoms out one level down.
    if (n.expanded && n.children) {
      result.push(...flattenTree(n.children));
    }
  }
  return result;
}

export function range(n: number): number[] {
  return Array.from({ length: n }, (_, i) => i);
}

export function countConnectionsInFolder(
  folderId: string,
  folders: Folder[],
  connections: Connection[]
): number {
  let count = connections.filter((c) => c.folderId === folderId).length;
  const children = folders.filter((f) => f.parentId === folderId);
  for (const child of children) {
    count += countConnectionsInFolder(child.id, folders, connections);
  }
  return count;
}
