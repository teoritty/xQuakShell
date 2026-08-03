<script lang="ts">
  // The tree's status light. One implementation, two callers: the connection
  // ping and discovery node status. The colour decision lives in statusDot.ts so
  // it can be tested without a DOM.
  import { statusDotColor, statusDotTooltip, type StatusDot } from './statusDot';

  /** null = nothing to show. The caller decides what occupies the slot instead. */
  export let status: StatusDot | null = null;
</script>

{#if status}
  <span class="ping-dot" style="background: {statusDotColor(status)}" title={statusDotTooltip(status)}></span>
{/if}
