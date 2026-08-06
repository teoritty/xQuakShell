// Shared data shape for the two file panes.
//
// FileTree.svelte works with RemoteNode and LocalFileTree.svelte with LocalNode.
// The panes' logic never cared about the difference — both are a name, a path,
// a directory flag and some optional columns — but because each pane spelled the
// type out for itself, every helper had to be written twice. FileNode is that
// shared shape, declared structurally so both node types satisfy it without
// either having to import the other.

import type { SortKey, SortDir } from '../filePanelToolbar';

export interface FileNode {
  path: string;
  name: string;
  isDir: boolean;
  size?: number;
  modTime?: string;
  mode?: string;
  owner?: string;
  /** Remote (SFTP) listings carry a group; local ones do not. */
  group?: string;
}

/** A pane's directory listings, keyed by directory path. */
export type FileTreeMap<T extends FileNode> = Map<string, T[]>;

/** The column sort a pane is currently applying, if any. */
export interface SortState {
  sortEnabled: boolean;
  sortKey: SortKey | null;
  sortDir: SortDir;
}
