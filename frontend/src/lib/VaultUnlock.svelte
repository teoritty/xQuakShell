<script lang="ts">
  // Vault gate: decides between creating a master password and unlocking with
  // an existing one. It lives here rather than in App.svelte because App mounts
  // synchronously and would render a screen before the answer arrives; owning
  // the probe here lets the gate show nothing until it knows.
  import { onMount } from 'svelte';
  import { vaultExists } from '../stores/appState';
  import { initVaultGate } from '../actions/vaultActions';
  import VaultCreateForm from './vault/VaultCreateForm.svelte';
  import VaultUnlockForm from './vault/VaultUnlockForm.svelte';

  onMount(() => {
    void initVaultGate();
  });
</script>

<div class="vault-screen">
  {#if $vaultExists === null}
    <!-- Probing. Rendering either card here would flash the wrong screen. -->
  {:else if $vaultExists}
    <VaultUnlockForm />
  {:else}
    <VaultCreateForm />
  {/if}
</div>

<style>
  .vault-screen {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: 1;
    padding: 24px;
    overflow: auto;
    background: var(--bg-primary);
  }
</style>
