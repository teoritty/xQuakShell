<script lang="ts">
  import type { KeyOptions } from '../../api/keys';

  export let options: KeyOptions;
  export let showNonExportable = true;
  // The passphrase-memory choice only means anything for a key that has a passphrase; for a
  // vault-protected key there is nothing to remember, and offering the choice would imply there is.
  export let hasPassphrase = true;
</script>

<fieldset>
  <legend>Settings</legend>

  {#if hasPassphrase}
    <label for="cache-policy">Remember the passphrase</label>
    <select id="cache-policy" bind:value={options.cachePolicy}>
      <option value="until-lock">Until the vault locks</option>
      <option value="duration">For a while</option>
      <option value="never">Never — ask every time</option>
    </select>

    {#if options.cachePolicy === 'duration'}
      <label for="cache-ttl">Minutes</label>
      <input id="cache-ttl" type="number" min="1" max="720" bind:value={options.cacheTtlSeconds} />
    {/if}
  {/if}

  <label class="check">
    <input type="checkbox" bind:checked={options.allowPlugins} />
    <span>
      Let plugins read this key
      <small>Off by default. A plugin granted vault access still cannot read this key unless you allow it here.</small>
    </span>
  </label>

  {#if showNonExportable}
    <label class="check">
      <input type="checkbox" bind:checked={options.nonExportable} />
      <span>
        Never let this key leave the vault
        <small>Permanent. This cannot be undone later — you would have to delete the key and make another.</small>
      </span>
    </label>
  {/if}
</fieldset>

<style>
  fieldset {
    margin: 10px 0 0;
    padding: 8px 10px 10px;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  legend {
    padding: 0 4px;
    font-size: 11px;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  label {
    font-size: 12px;
    color: var(--text-secondary);
  }


  .check {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    color: var(--text-primary);
    margin-top: 4px;
  }

  .check input {
    margin-top: 2px;
  }

  small {
    display: block;
    margin-top: 2px;
    font-size: 11px;
    line-height: 1.4;
    color: var(--text-secondary);
  }
</style>
