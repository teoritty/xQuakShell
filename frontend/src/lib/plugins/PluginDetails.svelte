<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Modal from '../Modal.svelte';
  import type { GitHubPluginMetadata } from '../../api/githubPlugins';
  import type { PluginSourceDTO } from '../../api/pluginSources';

  export let show = false;
  export let plugin: GitHubPluginMetadata | null = null;
  export let source: PluginSourceDTO | null = null;

  const dispatch = createEventDispatcher();

  $: platforms = plugin?.platforms ?? [];
  $: releases = plugin?.availableReleases ?? [];
</script>

<Modal
  title={plugin?.name || 'Plugin'}
  {show}
  contentClass="plugin-details-modal"
  on:close={() => dispatch('close')}
>
  {#if plugin}
    <div class="details">
      <dl class="facts">
        <dt>Identifier</dt><dd>{plugin.id}</dd>
        <dt>Version</dt><dd>{plugin.version || '—'}</dd>
        <dt>Author</dt><dd>{plugin.author || '—'}</dd>
        <dt>License</dt><dd>{plugin.license || '—'}</dd>
        <dt>Source</dt><dd>{source?.displayName || plugin.repositoryUrl}</dd>
        {#if plugin.installed}
          <dt>Installed</dt><dd>{plugin.installedVersion} ({plugin.installedReleaseTag || 'unknown tag'})</dd>
        {/if}
      </dl>

      {#if plugin.description}
        <p class="description">{plugin.description}</p>
      {/if}

      <section>
        <h5>Platforms</h5>
        {#if platforms.length === 0}
          <p class="note">This release publishes no platform assets.</p>
        {:else}
          <ul class="chips">
            {#each platforms as p}
              <li class="chip" class:current={p.os && plugin.platformSupported}>{p.os}/{p.arch}</li>
            {/each}
          </ul>
        {/if}
        {#if !plugin.platformSupported}
          <p class="note warn">No asset targets this machine, so this plugin cannot be installed here.</p>
        {/if}
      </section>

      <section>
        <h5>Releases</h5>
        {#if releases.length === 0}
          <p class="note">No published releases.</p>
        {:else}
          <ul class="releases">
            {#each releases.slice(0, 12) as release (release.tag)}
              <li>
                <span class="tag">{release.tag}</span>
                {#if release.prerelease}<span class="badge">pre-release</span>{/if}
                {#if !release.platformSupported}<span class="badge warn">unsupported here</span>{/if}
                <span class="date">{release.publishedAt}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      {#if plugin.readme}
        <section>
          <h5>Readme</h5>
          <!-- Rendered as text, never as markup: a readme is attacker-controlled content from a
               repository the user may not trust, and this window has the app's own privileges. -->
          <pre class="readme">{plugin.readme}</pre>
        </section>
      {/if}
    </div>
  {/if}

  <div class="dialog-actions">
    <button class="secondary" on:click={() => dispatch('close')}>Close</button>
  </div>
</Modal>

<style>
  .details {
    display: flex;
    flex-direction: column;
    gap: 14px;
    max-width: 68ch;
    max-height: 58vh;
    overflow-y: auto;
  }

  .facts {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 3px 14px;
    margin: 0;
    font-size: 12px;
  }

  .facts dt {
    color: var(--text-secondary);
  }

  .facts dd {
    margin: 0;
    overflow-wrap: anywhere;
  }

  .description {
    margin: 0;
    font-size: 12px;
  }

  h5 {
    margin: 0 0 5px;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-secondary);
  }

  .chips,
  .releases {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }

  .chip {
    font-size: 11px;
    padding: 2px 7px;
    border: 1px solid var(--border-color);
    border-radius: 10px;
  }

  .releases li {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 11.5px;
    padding: 2px 0;
  }

  .tag {
    font-weight: 600;
  }

  .badge {
    font-size: 10px;
    padding: 1px 6px;
    border-radius: 8px;
    background: var(--bg-secondary);
    color: var(--text-secondary);
  }

  .badge.warn {
    color: var(--warning);
  }

  .date {
    color: var(--text-secondary);
    margin-left: auto;
  }

  .note {
    margin: 0;
    font-size: 11.5px;
    color: var(--text-secondary);
  }

  .note.warn {
    color: var(--warning);
  }

  .readme {
    margin: 0;
    max-height: 220px;
    overflow: auto;
    padding: 8px 10px;
    border-radius: 5px;
    background: var(--bg-secondary);
    font-size: 11px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
