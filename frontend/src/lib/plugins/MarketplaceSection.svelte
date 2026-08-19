<script lang="ts">
  // The first-party registry, on its own page because it is not a source you can act on.
  //
  // It used to sit in the Sources list, where every other row offers trust, refresh and remove -
  // three controls that do nothing to a service that does not exist yet. A page that says what is
  // coming is honest; a disabled row among working ones just looks broken.
  import { createEventDispatcher } from 'svelte';
  import { Store, Plus } from 'lucide-svelte';
  import SectionHeading from './SectionHeading.svelte';
  import type { PluginSourceDTO } from '../../api/pluginSources';

  export let source: PluginSourceDTO | null = null;

  const dispatch = createEventDispatcher();

  // What the registry is for, written now so the page says something rather than only apologising.
  const promises = [
    'Search across every published plugin, instead of knowing a repository URL in advance.',
    'Versions resolved for your platform, so a listing never offers a build you cannot run.',
    'Publisher identity carried with the plugin, so signing means something across sources.',
  ];
</script>

<SectionHeading
  title="Marketplace"
  subtitle="A first-party place to find plugins, without having to know where they are hosted."
/>

<div class="soon">
  <div class="soon-head">
    <span class="soon-icon"><Store size={17} /></span>
    <div>
      <div class="soon-title">Coming soon</div>
      <div class="soon-host">{source?.id ?? 'api.xquakshell.ru'}</div>
    </div>
  </div>

  <p class="soon-body">
    {source?.unavailableReason ?? 'Nothing is serving plugins here yet.'}
    This build does not contact the address above.
  </p>

  <ul class="soon-list">
    {#each promises as promise}
      <li>{promise}</li>
    {/each}
  </ul>

  <div class="soon-foot">
    <p class="soon-note">
      Whatever the marketplace serves will pass the same manifest validation, signature policy and
      permission prompts as a plugin from any repository. Being first-party is not a reason to skip
      a check.
    </p>
    <button class="secondary small" on:click={() => dispatch('goToSources')}>
      <Plus size={12} />
      Add a repository instead
    </button>
  </div>
</div>

<style>
  .soon {
    padding: 20px;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    background: var(--bg-primary);
    max-width: 62ch;
  }

  .soon-head {
    display: flex;
    align-items: center;
    gap: 11px;
    margin-bottom: 12px;
  }

  .soon-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 34px;
    height: 34px;
    border-radius: 6px;
    background: var(--accent-muted);
    color: var(--accent);
  }

  .soon-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-bright);
  }

  .soon-host {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-secondary);
  }

  .soon-body {
    margin: 0 0 12px;
    font-size: 12px;
    line-height: 1.55;
    color: var(--text-primary);
  }

  .soon-list {
    margin: 0 0 16px;
    padding-left: 17px;
    display: flex;
    flex-direction: column;
    gap: 5px;
    font-size: 12px;
    line-height: 1.5;
    color: var(--text-secondary);
  }

  .soon-foot {
    padding-top: 13px;
    border-top: 1px solid var(--border-color);
  }

  .soon-note {
    margin: 0 0 12px;
    font-size: 11.5px;
    line-height: 1.5;
    color: var(--text-secondary);
  }

  .small {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    padding: 4px 9px;
  }
</style>
