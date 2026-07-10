import type { ConnectionUser, ForwardRule, JumpHop } from '../../stores/appState';

export type SaveStatus = 'idle' | 'saving' | 'saved';

export type AuthMethod = 'key' | 'password' | 'plugin';

/** Local editable state for the connection details panel. */
export interface ConnectionDetailsDraft {
  editingId: string;
  name: string;
  protocol: string;
  host: string;
  port: number;
  tags: string[];
  users: ConnectionUser[];
  defaultUserId: string;
  jumpHops: JumpHop[];
  forwardRules: ForwardRule[];
  pluginFields: Record<string, unknown>;
}
