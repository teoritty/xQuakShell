import type { Connection, Folder } from '../../stores/appState';

export type ConnectionStatus = 'active' | 'connecting' | 'error' | 'disconnected';
export type DropZone = 'folder' | 'before' | 'after';

export interface TreeNode {
  type: 'folder' | 'connection';
  id: string;
  name: string;
  depth: number;
  parentId: string;
  folder?: Folder;
  connection?: Connection;
  children?: TreeNode[];
  expanded?: boolean;
  tags?: string[];
}

export interface DragPayload {
  folderIds: string[];
  connectionIds: string[];
}

export interface DragVisualState {
  dragOverId: string | null;
  dragOverRoot: boolean;
  dragOverDropZone: DropZone | null;
  dragOverTargetId: string | null;
}

export const emptyDragPayload = (): DragPayload => ({
  folderIds: [],
  connectionIds: [],
});

export const emptyDragVisualState = (): DragVisualState => ({
  dragOverId: null,
  dragOverRoot: false,
  dragOverDropZone: null,
  dragOverTargetId: null,
});
