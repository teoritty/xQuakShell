<script lang="ts">
  /** Collapsible list of things the importer did not honour. */
  import { AlertTriangle, ChevronDown, ChevronRight } from 'lucide-svelte';
  import type { SSHConfigNotice } from '../../api/sshConfig';
  import { describeNotice } from './importSelection';

  export let notices: SSHConfigNotice[] = [];

  // Collapsed by default: notices are informational, and an import that
  // mostly worked should not open looking like a failure.
  let expanded = false;
</script>

{#if notices.length > 0}
  <div class="notices">
    <button class="toggle" on:click={() => (expanded = !expanded)} aria-expanded={expanded}>
      {#if expanded}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}
      <AlertTriangle size={12} />
      <span>
        {notices.length}
        {notices.length === 1 ? 'thing was' : 'things were'} not imported as written
      </span>
    </button>
    {#if expanded}
      <ul>
        {#each notices as notice, i (notice.kind + notice.target + i)}
          <li>{describeNotice(notice)}</li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}

<style>
  .notices {
    border: 1px solid var(--border-color);
    border-radius: 4px;
    padding: 6px 8px;
  }

  .toggle {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 0;
    background: none;
    border: none;
    color: var(--text-secondary);
    font-size: 11px;
    text-align: left;
    cursor: pointer;
  }

  .toggle:hover {
    color: var(--text-primary);
  }

  ul {
    margin: 6px 0 0;
    padding-left: 18px;
  }

  li {
    font-size: 11px;
    color: var(--text-secondary);
    margin-bottom: 3px;
  }
</style>
