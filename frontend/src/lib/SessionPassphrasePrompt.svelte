<script lang="ts">
  import SessionPassphraseDialog from './SessionPassphraseDialog.svelte';
  import { pendingPassphrasePrompts, dropPassphrasePrompt } from '../stores/passphrasePromptState';
  import { resolvePassphraseRpc, cancelPassphraseRpc } from '../api/sessions';

  // Container for the key passphrase prompt, split the way PeerTrustPrompt is: this owns the queue
  // and the answer, the dialog owns only how it looks. App.svelte mounts it once.

  // Only the head of the queue is on screen. Stacked modals would invite typing a passphrase into
  // whichever happened to be on top, for a key the user was not looking at.
  $: prompt = $pendingPassphrasePrompts[0] ?? null;

  // The dialog closes once the backend has taken the passphrase. A failed hand-over keeps it up so
  // the user can try again; the backend withdraws a prompt that can no longer be answered with its
  // own PassphrasePromptClosed event, so a stale dialog does not linger either way.
  async function submit(requestId: string, passphrase: string) {
    if (await resolvePassphraseRpc(requestId, passphrase)) {
      dropPassphrasePrompt(requestId);
    }
  }

  // Cancel always takes the dialog down. The only refusal the backend has for it is "nothing is
  // waiting on this id", and then there is no connection left for the dialog to belong to.
  async function cancel(requestId: string) {
    await cancelPassphraseRpc(requestId);
    dropPassphrasePrompt(requestId);
  }
</script>

{#if prompt}
  {#key prompt.requestId}
    <SessionPassphraseDialog
      show={true}
      label={prompt.label}
      on:submit={(e) => prompt && submit(prompt.requestId, e.detail.passphrase)}
      on:cancel={() => prompt && cancel(prompt.requestId)}
    />
  {/key}
{/if}
