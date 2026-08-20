<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Activity, Power, Trash2 } from 'lucide-svelte';
  import PluginCard from './PluginCard.svelte';
  import SectionHeading from './SectionHeading.svelte';
  import { groupInstalled, isBundled, pluginStateLabel, sandboxSummary } from './pluginsView';
  import type { PluginInfo } from '../../api/plugins';
  import { t } from '../../i18n/messages';

  export let plugins: PluginInfo[] = [];
  export let query = '';
  export let busyPluginId = '';

  const dispatch = createEventDispatcher();

  $: groups = groupInstalled(plugins, query);
  $: matched = groups.reduce((n, g) => n + g.plugins.length, 0);

  // The signature and sandbox chips say how much this plugin can be believed and how contained it
  // is, so they take their words from the security namespace: a pack on disk must not be able to
  // relabel an unsigned plugin as signed.
  function chipsFor(plugin: PluginInfo) {
    const chips: { label: string; tone: 'good' | 'warn' | 'bad' | 'neutral' }[] = [];
    chips.push(
      plugin.signed
        ? { label: $t('security.plugin.chip.signed'), tone: 'good' }
        : { label: $t('security.plugin.chip.unsigned'), tone: 'warn' },
    );
    const sandbox = sandboxSummary(plugin.sandboxMode);
    if (sandbox) chips.push({ label: $t(sandbox.labelKey), tone: sandbox.tone });
    if (plugin.requiresSecretAccess) {
      chips.push({ label: $t('security.plugin.chip.readsSecrets'), tone: 'warn' });
    }
    // Only the bundled case earns a chip. The other value of this field is "user", which is true of
    // everything the user can see here and so tells them nothing.
    if (isBundled(plugin)) chips.push({ label: $t('plugins.chip.bundled'), tone: 'neutral' });
    return chips;
  }

  function statusFor(plugin: PluginInfo): { text: string; kind: 'installed' | 'warning' } {
    if (!plugin.enabled) return { text: $t('plugins.state.disabled'), kind: 'warning' };
    if (plugin.state === 'running') return { text: $t('plugins.state.running'), kind: 'installed' };
    const state = pluginStateLabel(plugin.state);
    return { text: state.key ? $t(state.key) : (state.text ?? ''), kind: 'warning' };
  }
</script>

<SectionHeading
  title={$t('plugins.section.installed')}
  subtitle={$t('plugins.installed.subtitle')}
  count={query ? $t('plugins.count.matched', { matched, total: plugins.length }) : ''}
/>

{#if plugins.length === 0}
  <div class="empty">
    <p class="empty-title">{$t('plugins.installed.empty.title')}</p>
    <p class="empty-body">{$t('plugins.installed.empty.body')}</p>
  </div>
{:else if groups.length === 0}
  <p class="no-match">{$t('plugins.noMatch', { query })}</p>
{:else}
  {#each groups as group (group.id)}
    <section class="group">
      <header class="group-head">
        <span class="group-label" class:attention={group.id === 'attention'}>{$t(group.labelKey)}</span>
        <span class="group-count">{group.plugins.length}</span>
        <span class="group-hint">{$t(group.hintKey)}</span>
      </header>

      <div class="card-list">
        {#each group.plugins as plugin (plugin.id)}
          {@const status = statusFor(plugin)}
          <PluginCard
            name={plugin.name}
            version={plugin.version}
            description={plugin.description}
            state={group.id === 'disabled' ? 'idle' : group.id}
            chips={chipsFor(plugin)}
            status={status.text}
            statusKind={status.kind}
            dimmed={!plugin.enabled}
          >
            <svelte:fragment slot="actions">
              <button
                class="ghost icon-btn"
                class:on={plugin.enabled}
                title={plugin.enabled ? $t('plugins.action.disable') : $t('plugins.action.enable')}
                disabled={busyPluginId === plugin.id}
                on:click={() => dispatch('toggle', { plugin, enabled: !plugin.enabled })}
              >
                <Power size={13} />
              </button>
              <button
                class="ghost icon-btn"
                title={$t('plugins.action.ping')}
                disabled={busyPluginId === plugin.id || !plugin.enabled}
                on:click={() => dispatch('ping', { plugin })}
              >
                <Activity size={13} />
              </button>
              <button
                class="ghost icon-btn danger"
                title={$t('plugins.action.uninstall')}
                disabled={busyPluginId === plugin.id}
                on:click={() => dispatch('uninstall', { plugin })}
              >
                <Trash2 size={13} />
              </button>
            </svelte:fragment>
          </PluginCard>
        {/each}
      </div>
    </section>
  {/each}
{/if}

<style>
  .group {
    margin-bottom: 18px;
  }

  .group-head {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin-bottom: 7px;
    padding-bottom: 5px;
    border-bottom: 1px solid var(--border-color);
  }

  .group-label {
    font-size: 11px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-secondary);
  }

  .group-label.attention {
    color: var(--warning);
  }

  .group-count {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-secondary);
  }

  .group-hint {
    font-size: 11px;
    color: var(--text-secondary);
    margin-left: auto;
  }

  .card-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .empty {
    padding: 26px 20px;
    border: 1px dashed var(--border-color);
    border-radius: 6px;
    text-align: center;
  }

  .empty-title {
    margin: 0 0 5px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-bright);
  }

  .empty-body {
    margin: 0 auto;
    max-width: 46ch;
    font-size: 12px;
    line-height: 1.55;
    color: var(--text-secondary);
  }

  .no-match {
    margin: 0;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .icon-btn {
    display: inline-flex;
    align-items: center;
    padding: 3px;
  }

  .icon-btn.on {
    color: var(--success);
  }

  .icon-btn.danger:hover:not(:disabled) {
    color: var(--danger);
  }
</style>
