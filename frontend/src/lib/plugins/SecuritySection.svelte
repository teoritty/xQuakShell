<script lang="ts">
  // The trust policy, unchanged: PluginTrustPolicy owns loading, saving and the master-password
  // re-auth the backend demands for a weakening change. It is reused rather than rebuilt because
  // that re-auth path is security-critical and already correct - the backend decides when a
  // password is required, and a second copy of that rule here is the one that would drift.
  import PluginTrustPolicy from '../PluginTrustPolicy.svelte';
  import SectionHeading from './SectionHeading.svelte';

  export let onError: (message: string) => void = () => {};
</script>

<SectionHeading
  title="Security"
  subtitle="Rules applied to every plugin, whatever source it came from. Checked when a plugin installs and again every time it starts."
/>

<PluginTrustPolicy {onError} />
