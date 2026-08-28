<script lang="ts">
  // The plugin trust policy as a Settings section. It moved here from the Plugins screen because
  // it is not a fact about any installed plugin: it is what this installation demands before any
  // plugin runs, which is the same question the lockout above it answers for the vault.
  //
  // The error line is owned here rather than passed further up: PluginTrustPolicy saves on every
  // toggle, outside the dialog's Save button, so a failure has to be reported where it happened
  // and not in a footer the user may never press.
  import PluginTrustPolicy from '../PluginTrustPolicy.svelte';

  let errorMessage = '';
</script>

<div class="section">
  <h4>Plugin trust policy</h4>
  <p class="section-desc">
    Applied to every plugin, whatever source it came from. Checked when a plugin installs and again
    every time it starts. These controls save immediately.
  </p>
  {#if errorMessage}
    <p class="trust-error">{errorMessage}</p>
  {/if}
  <PluginTrustPolicy onError={(message) => (errorMessage = message)} />
</div>

<style>
  .trust-error {
    margin: 0;
    font-size: 11px;
    line-height: 1.45;
    color: var(--danger);
  }
</style>
