import type { ConnectionProtocol } from '../../actions/protocolActions';
import type { ConnectionDetailsDraft } from './types';
import {
  applyProtocolFieldDefaults,
  refreshFormModeFromDraft,
  resolveConnectionFormMode,
} from './connectionFormMode';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const sshOnlyCatalog: ConnectionProtocol[] = [
  { id: 'ssh', label: 'SSH', defaultPort: 22 },
];

const catalogWithTelnet: ConnectionProtocol[] = [
  ...sshOnlyCatalog,
  {
    id: 'telnet',
    label: 'Telnet',
    defaultPort: 23,
    fields: [
      {
        id: 'auth',
        label: 'Auth',
        order: 0,
        fields: [
          { id: 'username', label: 'Username', type: 'text', order: 0, required: false, secret: false, default: 'root' },
        ],
      },
    ],
  },
];

const sshMode = resolveConnectionFormMode('ssh', catalogWithTelnet);
assert(sshMode.mode === 'ssh', 'ssh protocol resolves to ssh mode');
assert(sshMode.protocolDef?.id === 'ssh', 'ssh mode includes ssh protocol def');

const telnetBeforeCatalog = resolveConnectionFormMode('telnet', sshOnlyCatalog);
assert(telnetBeforeCatalog.mode === 'none', 'telnet before catalog load resolves to none');
assert(telnetBeforeCatalog.protocolDef === null, 'telnet before catalog has no protocol def');

const telnetAfterCatalog = resolveConnectionFormMode('telnet', catalogWithTelnet);
assert(telnetAfterCatalog.mode === 'plugin', 'telnet with fields resolves to plugin');
assert(telnetAfterCatalog.protocolDef?.id === 'telnet', 'plugin mode includes telnet protocol def');

const unknown = resolveConnectionFormMode('rdp', sshOnlyCatalog);
assert(unknown.mode === 'none', 'unknown protocol without fields resolves to none');

const draft: ConnectionDetailsDraft = {
  editingId: 'c1',
  name: 'Telnet host',
  protocol: 'telnet',
  host: '10.0.0.1',
  port: 23,
  tags: [],
  users: [],
  defaultUserId: '',
  jumpHops: [],
  forwardRules: [],
  pluginFields: {},
  storedSecretFields: [],
  pluginFieldsTouched: {},
};

applyProtocolFieldDefaults(draft, 'telnet', catalogWithTelnet);
assert(draft.pluginFields.username === 'root', 'defaults applied for explicit protocol id');

const staleDraft: ConnectionDetailsDraft = {
  ...draft,
  protocol: 'telnet',
  pluginFields: {},
  storedSecretFields: [],
  pluginFieldsTouched: {},
};
applyProtocolFieldDefaults(staleDraft, 'ssh', catalogWithTelnet);
assert(staleDraft.pluginFields.username === undefined, 'defaults use passed protocol id, not draft.protocol');

const refreshed = refreshFormModeFromDraft(
  { ...draft, protocol: 'telnet' },
  catalogWithTelnet,
);
assert(refreshed.mode === 'plugin', 'refreshFormModeFromDraft returns plugin for telnet draft');

console.log('connectionFormMode.test.ts: all passed');
