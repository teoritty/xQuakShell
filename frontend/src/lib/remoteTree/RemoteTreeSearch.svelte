<script lang="ts">
  import { Search } from 'lucide-svelte';
  import './remoteTreeShared.css';

  export let value = '';
  export let onFocus: (() => void) | undefined = undefined;
</script>

<div class="search-bar">
  <Search size={12} class="search-icon" />
  <input
    type="text"
    class="search-input"
    placeholder="Search connections..."
    bind:value
    on:focus={() => onFocus?.()}
  />
</div>
