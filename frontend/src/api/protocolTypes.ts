// Connection-protocol domain types. Pure type definitions, no logic — this
// lives in the api/ layer so both api/* (e.g. pluginRuntime.ts) and
// actions/* (e.g. protocolActions.ts) can depend on it without inverting the
// intended api -> actions dependency direction.

export interface FieldDef {
  id: string;
  label: string;
  // keyValue and code were added to the shared field schema by ADR-015, so a connection protocol
  // may declare them too — the host validates both here as it does in a dialog.
  type: 'text' | 'password' | 'number' | 'select' | 'checkbox' | 'textarea' | 'keyValue' | 'code';
  required: boolean;
  default?: unknown;
  placeholder?: string;
  description?: string;
  width?: 'full' | 'half' | 'third';
  order: number;
  validation?: {
    minLength?: number;
    maxLength?: number;
    min?: number;
    max?: number;
    pattern?: string;
    maxSizeBytes?: number;
  };
  options?: { value: string; label: string }[];
  dependsOn?: string;
  secret: boolean;
}

export interface FieldGroup {
  id: string;
  label: string;
  order: number;
  fields: FieldDef[];
}

export interface ConnectionProtocol {
  id: string;
  label: string;
  defaultPort?: number;
  icon?: string;
  surface?: 'terminal' | 'embed';
  remoteFs?: boolean;
  fields?: FieldGroup[];
}
