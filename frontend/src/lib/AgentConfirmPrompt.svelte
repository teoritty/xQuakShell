<script lang="ts">
  import AgentConfirmDialog from './AgentConfirmDialog.svelte';
  import { pendingAgentConfirms, dropAgentConfirm } from '../stores/agentConfirmState';
  import { resolveAgentConfirmRpc, type AgentDecision } from '../api/agentForwarding';

  // Container for forwarded-agent confirmations, split the way SessionPassphrasePrompt is: this
  // owns the queue and the answer, the dialog owns only how it looks. App.svelte mounts it once.

  // Only the head of the queue is on screen, so a click always answers the request the user is
  // reading - stacked dialogs would let one approval land on a request hidden underneath.
  $: request = $pendingAgentConfirms[0] ?? null;

  // The dialog leaves whatever the backend said: an answer it refused belongs to a request that is
  // no longer waiting (timed out, session closed), and the backend has already denied it.
  async function decide(sessionId: string, requestId: string, decision: AgentDecision) {
    await resolveAgentConfirmRpc(sessionId, requestId, decision);
    dropAgentConfirm(requestId);
  }
</script>

{#if request}
  {#key request.requestId}
    <AgentConfirmDialog
      {request}
      on:decide={(e) => request && decide(request.sessionId, request.requestId, e.detail)}
    />
  {/key}
{/if}
