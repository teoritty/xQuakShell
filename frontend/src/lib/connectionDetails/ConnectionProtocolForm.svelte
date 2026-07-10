<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { ConnectionProtocol } from '../../stores/api';
  import type { ConnectionFormMode } from './connectionFormMode';
  import type { ConnectionUser, ForwardRule, JumpHop, SSHIdentityMeta } from '../../stores/appState';
  import ConnectionUsers from './ConnectionUsers.svelte';
  import JumpHosts from './JumpHosts.svelte';
  import ForwardRules from './ForwardRules.svelte';
  import PluginConnectionFields from './PluginConnectionFields.svelte';

  export let mode: ConnectionFormMode = 'none';
  export let protocolDef: ConnectionProtocol | null = null;
  export let users: ConnectionUser[] = [];
  export let defaultUserId = '';
  export let jumpHops: JumpHop[] = [];
  export let forwardRules: ForwardRule[] = [];
  export let identities: SSHIdentityMeta[] = [];
  export let pluginFields: Record<string, unknown> = {};
  export let fieldErrors: Record<string, string> = {};

  const dispatch = createEventDispatcher<{
    dirty: void;
    userschange: ConnectionUser[];
    defaultuserchange: string;
    hopschange: JumpHop[];
    forwardruleschange: ForwardRule[];
    keyimport: string;
    keyremove: { userId?: string; hopId?: string; keyId: string };
    passwordchange: { userId?: string; hopId?: string; value: string };
    fieldchange: { fieldId: string; value: unknown };
  }>();
</script>

{#if mode === 'ssh'}
  <ConnectionUsers
    {users}
    {defaultUserId}
    {identities}
    on:dirty={() => dispatch('dirty')}
    on:userschange={(e) => dispatch('userschange', e.detail)}
    on:defaultuserchange={(e) => dispatch('defaultuserchange', e.detail)}
    on:keyimport={(e) => dispatch('keyimport', e.detail)}
    on:keyremove={(e) => dispatch('keyremove', e.detail)}
    on:passwordchange={(e) => dispatch('passwordchange', e.detail)}
  />

  <JumpHosts
    {jumpHops}
    {identities}
    on:dirty={() => dispatch('dirty')}
    on:hopschange={(e) => dispatch('hopschange', e.detail)}
    on:keyimport={(e) => dispatch('keyimport', e.detail)}
    on:keyremove={(e) => dispatch('keyremove', e.detail)}
    on:passwordchange={(e) => dispatch('passwordchange', e.detail)}
  />

  <ForwardRules
    rules={forwardRules}
    {fieldErrors}
    on:dirty={() => dispatch('dirty')}
    on:ruleschange={(e) => dispatch('forwardruleschange', e.detail)}
  />
{:else if mode === 'plugin' && protocolDef?.fields}
  <PluginConnectionFields
    groups={protocolDef.fields}
    bind:values={pluginFields}
    bind:errors={fieldErrors}
    on:fieldchange={(e) => dispatch('fieldchange', e.detail)}
  />
{/if}
