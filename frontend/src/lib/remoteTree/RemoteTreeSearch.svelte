<script lang="ts">
  import { t } from '../../i18n/messages';
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
    placeholder={$t('tree.search.placeholder')}
    bind:value
    on:focus={() => onFocus?.()}
  />
</div>
