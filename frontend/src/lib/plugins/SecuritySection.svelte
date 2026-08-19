<script lang="ts">
  // The trust policy, unchanged: PluginTrustPolicy owns loading, saving and the master-password
  // re-auth the backend demands for a weakening change. It is reused rather than rebuilt because
  // that re-auth path is security-critical and already correct - the backend decides when a
  // password is required, and a second copy of that rule here is the one that would drift.
  import PluginTrustPolicy from '../PluginTrustPolicy.svelte';

  export let onError: (message: string) => void = () => {};
</script>

<p class="hint">
  These settings apply to every plugin, whatever source it came from. They are checked at install
  time and again at every start.
</p>

<PluginTrustPolicy {onError} />

<style>
  .hint {
    margin: 0 0 12px;
    font-size: 11px;
    color: var(--text-secondary);
    max-width: 60ch;
  }
</style>
