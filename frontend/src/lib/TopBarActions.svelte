<script lang="ts">
  // The top bar's icon buttons, in one place.
  //
  // Extracted from App.svelte when adding Plugins pushed that file past its size budget. It is a
  // real seam rather than a way to move lines: every button here opens a global manager window,
  // none of them touches session or layout state, and App was holding seven near-identical
  // markup blocks plus their styles for no reason beyond history.
  //
  // It dispatches rather than binding the flags, so App keeps ownership of which dialog is open.
  import { createEventDispatcher } from 'svelte';
  import { Settings, FileText, Shield, Fingerprint, Terminal, Key, Puzzle } from 'lucide-svelte';

  const dispatch = createEventDispatcher();

  // Order is deliberate: the two most-used managers first, then the trust ones, then Plugins and
  // Settings pinned to the right edge where they were before.
  const actions = [
    { event: 'scripts', title: 'Scripts', icon: Terminal },
    { event: 'audit', title: 'Audit Log', icon: FileText },
    { event: 'knownHosts', title: 'Known Hosts', icon: Shield },
    { event: 'peerTrust', title: 'Trusted Peers', icon: Fingerprint },
    { event: 'keys', title: 'SSH Keys', icon: Key },
    { event: 'plugins', title: 'Plugins', icon: Puzzle },
    { event: 'settings', title: 'Settings', icon: Settings },
  ];
</script>

<div class="top-bar-actions">
  {#each actions as action (action.event)}
    <button class="ghost top-btn" title={action.title} on:click={() => dispatch(action.event)}>
      <svelte:component this={action.icon} size={14} />
    </button>
  {/each}
</div>

<style>
  .top-bar-actions {
    display: flex;
    align-items: center;
    padding: 0 4px;
    gap: 1px;
    flex-shrink: 0;
  }

  .top-btn {
    padding: 4px 6px;
    border-radius: 2px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
</style>
