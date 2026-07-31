<script lang="ts">
  // Renders a plugin-supplied discovery icon.
  //
  // THE ONLY REASON THIS COMPONENT EXISTS is the <img> below. A discovery icon
  // is a data URI built from a file inside the plugin's bundle, and SVG is an
  // allowed extension. Inlining that markup — {@html}, innerHTML, anything that
  // parses it as part of this document — would execute scripts from the plugin's
  // bundle inside the main window, with the app's full DOM in reach. Loaded
  // through <img src="data:...">, the same bytes render as an image and nothing
  // in them runs. Do not "simplify" this into an inline SVG.
  //
  // Nothing validates the file's CONTENTS on the way here either: install-time
  // validation checks the extension, so bytes that are not an image at all can
  // reach this component under a .svg name. That is what on:error is for — a
  // broken icon must degrade to the fallback glyph, not to an empty gap or a
  // browser's broken-image marker.
  import { Box } from 'lucide-svelte';

  /** Already-encoded data URI, or '' when the plugin named no icon. */
  export let src = '';
  export let label = '';
  export let size = 14;

  let failed = false;
  // Re-arm the fallback when the row is reused for a different icon.
  $: if (src) failed = false;
</script>

<span class="conn-icon plugin-icon" style="width: {size}px; height: {size}px">
  {#if src && !failed}
    <!-- Written out rather than using the {src} shorthand so that this line is
         greppable, and so discoveryMarkup.test.ts can assert on it. -->
    <img src={src} alt={label} width={size} height={size} on:error={() => (failed = true)} />
  {:else}
    <Box {size} />
  {/if}
</span>

<style>
  .plugin-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .plugin-icon img {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
  }
</style>
