<script lang="ts">
  // Marks a tab whose shell has a forwarded agent, so it is always visible which servers can ask
  // for a signature (ADR-022). It follows the saved connection: switching forwarding off there takes
  // effect at the server's next request, and the badge goes with it. A shell opened before
  // forwarding was switched on has no agent until it reconnects; the badge errs toward showing one
  // there, because a mark that hides a live agent is the mistake that matters.
  import { KeyRound } from 'lucide-svelte';
  import { t } from '../../i18n/messages';
  import { sessions, connections } from '../../stores/appState';

  export let id: string;

  $: session = $sessions.find((s) => s.sessionId === id);
  $: forwarding = (() => {
    if (!session || (session.protocol && session.protocol !== 'ssh')) return false;
    const af = $connections.find((c) => c.id === session.connectionId)?.agentForwarding;
    return af?.enabled === true && af.identityIds.length > 0;
  })();
</script>

{#if forwarding}
  <span class="agent-badge" title={$t('security.agent.tabBadge')}><KeyRound size={11} /></span>
{/if}

<style>
  .agent-badge { display: inline-flex; align-items: center; flex-shrink: 0; color: var(--accent); }
</style>
