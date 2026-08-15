<script lang="ts">
  import PeerTrustDialog from './PeerTrustDialog.svelte';
  import { sessions, pendingPeerTrust, dropPeerTrust } from '../stores/appState';
  import { resolvePeerTrustRpc } from '../api/sessions';

  // Container for the peer trust dialog: it owns the pending questions and the decision, the
  // dialog below owns only how it looks. App.svelte mounts this and knows nothing else about it -
  // the same split the host key dialog would get if it were written today.

  // The head of the queue, and only ever one dialog on screen. Two sessions can each be waiting on
  // a question; stacking their modals would put the second over the first and invite an answer to
  // whichever happens to be on top.
  $: prompt = $pendingPeerTrust[0] ?? null;
  $: connectionName =
    $sessions.find((s) => s.sessionId === prompt?.sessionId)?.connectionName ?? '';

  // A question whose session is gone is dropped rather than answered: the session closed, or
  // failed, and asking about it now would be asking about nothing. Guarded on a non-empty session
  // list so the first frame - before sessions have loaded - does not clear a live question.
  $: if ($sessions.length > 0) {
    for (const q of $pendingPeerTrust) {
      if (!$sessions.some((s) => s.sessionId === q.sessionId)) dropPeerTrust(q.sessionId);
    }
  }

  // The refusal goes to the backend too, not just to this store: the session holds the pending
  // decision, and a question dropped only here would leave it waiting on an answer that already
  // happened. The dialog closes only once the backend has taken the answer.
  //
  // The fingerprint travels back with the decision so the backend can refuse an answer that no
  // longer matches what was on screen.
  async function decide(action: 'trust' | 'reject') {
    if (!prompt) return;
    const { sessionId, fingerprint } = prompt;
    if (await resolvePeerTrustRpc(sessionId, action, fingerprint)) {
      dropPeerTrust(sessionId);
    }
  }
</script>

{#if prompt}
  <PeerTrustDialog
    show={true}
    {connectionName}
    subject={prompt.subject}
    fingerprint={prompt.fingerprint}
    isMismatch={prompt.mismatch}
    on:trust={() => decide('trust')}
    on:reject={() => decide('reject')}
  />
{/if}
