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
  // Plugin field ids that already have a secret stored in the vault (masked to "" in pluginFields).
  storedSecretFields: string[];
  // Plugin field ids the user has actually edited this session. A stored secret that is NOT touched
  // is left out of the save payload so the backend keeps it; touching it (even to clear) sends it.
  pluginFieldsTouched: Record<string, boolean>;
}
