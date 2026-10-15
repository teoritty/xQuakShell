<script lang="ts">
  // Plugins on the Sync page (spec 6.4-6.5): the plugins the user's other devices recorded, and the
  // plugins whose data follows the user. Nothing here installs or uninstalls by itself - every
  // install goes through the same consent screen as the Plugins screen, and an uninstall is a
  // button pressed twice because it is the one action that can lose data. Offers, conflicts and
  // updates waiting for a restart show whenever there are any; the per-plugin switches only with
  // the advanced settings, since syncing a plugin's data is the default and rarely changed.
  import InstallFlow from '../../plugins/InstallFlow.svelte';
  import type { ConsentAnswers } from '../../plugins/installConsent';
  import { offerActions, offerMessageKey } from './pluginOffers';
  import {
    installOfferedPlugin,
    keepPlugin,
    loadPluginSync,
    restartPluginForData,
    setPluginDataSync,
  } from '../../../actions/syncPluginActions';
  import { removePlugin } from '../../../actions/pluginsActions';
  import { errorMessage } from '../../../actions/syncActions';
  import type { SyncPluginData, SyncPluginOffer } from '../../../api/syncPlugins';
  import { dismissPluginDataNotice, pluginDataNotices, syncRevision } from '../../../stores/syncState';
  import { t } from '../../../i18n/messages';
  import { showError } from '../../../stores/appState';

  export let advanced = false;

  let offers: SyncPluginOffer[] = [];
  let data: SyncPluginData[] = [];
  let installFlow: InstallFlow;
  let mismatch: { pluginId: string; answers: ConsentAnswers } | null = null;
  let uninstalling = '';
  let busy = false;
  let error = '';

  $: needsYou = $pluginDataNotices.length > 0 || data.some((d) => d.waiting > 0);
  $: void load($syncRevision);

  async function load(..._deps: unknown[]) {
    const res = await loadPluginSync();
    offers = res.offers;
    data = res.data;
    error = res.error;
  }

  async function run(fn: () => Promise<string>) {
    busy = true;
    error = await fn();
    busy = false;
    await load();
  }

  async function act(offer: SyncPluginOffer, action: string) {
    if (action === 'install') return installFlow.fromOffer(offer.pluginId);
    if (action === 'keep') return run(() => keepPlugin(offer.pluginId));
    if (uninstalling !== offer.pluginId) {
      uninstalling = offer.pluginId;
      return;
    }
    uninstalling = '';
    await run(async () => {
      try {
        await removePlugin(offer.pluginId, false);
        return '';
      } catch (e) {
        return errorMessage(e);
      }
    });
  }

  async function installAnyway() {
    if (!mismatch) return;
    const { pluginId, answers } = mismatch;
    mismatch = null;
    await run(async () => (await installOfferedPlugin(pluginId, answers, true)).error);
  }

  function actionLabel(offer: SyncPluginOffer, action: string): string {
    if (action === 'uninstall' && uninstalling === offer.pluginId) return $t('sync.plugins.uninstallConfirm');
    return $t(`sync.plugins.${action}`);
  }

  const nameOf = (id: string) => data.find((d) => d.pluginId === id)?.name || id;
</script>

{#if offers.length > 0 || mismatch}
<div class="section">
  <h4>{$t('sync.plugins.offers.title')}</h4>
  {#if offers.length > 0}
    <ul class="sync-list">
      {#each offers as o (o.pluginId)}
        <li>
          <div>{o.pluginId} {o.version}</div>
          <div class="sync-meta">{$t(offerMessageKey(o), { repo: o.repo ?? '', tag: o.tag ?? '' })}</div>
          {#if offerActions(o).length > 0}
            <div class="sync-actions">
              {#each offerActions(o) as action}
                <button type="button" class={action === 'uninstall' ? 'danger' : 'secondary'} disabled={busy}
                  on:click={() => act(o, action)}>{actionLabel(o, action)}</button>
              {/each}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
  {#if mismatch}
    <p class="sync-hint" role="alert">{$t('sync.plugins.mismatch')}</p>
    <div class="sync-actions">
      <button type="button" class="danger" on:click={installAnyway} disabled={busy}>{$t('sync.plugins.installAnyway')}</button>
      <button type="button" class="secondary" on:click={() => (mismatch = null)} disabled={busy}>{$t('sync.plugins.cancel')}</button>
    </div>
  {/if}
</div>
{/if}

{#if advanced || needsYou}
<div class="section">
  <h4>{$t('sync.plugins.data.title')}</h4>
  {#each $pluginDataNotices as n (n.pluginId)}
    <div class="sync-notice" role="status">
      {#if n.conflicts.length > 0}
        <p class="sync-hint">{$t('sync.plugins.data.conflicts', { plugin: nameOf(n.pluginId), files: n.conflicts.join(', ') })}</p>
      {/if}
      {#if n.skipped.length > 0}
        <p class="sync-hint">{$t('sync.plugins.data.skipped', { plugin: nameOf(n.pluginId), files: n.skipped.join(', ') })}</p>
      {/if}
      <button type="button" class="secondary" on:click={() => dismissPluginDataNotice(n.pluginId)}>{$t('sync.plugins.dismiss')}</button>
    </div>
  {/each}
  {#if data.length === 0}
    <p class="sync-hint">{$t('sync.plugins.data.none')}</p>
  {:else}
    <ul class="sync-list">
      {#each data as d (d.pluginId)}
        <li>
          <label>
            <input type="checkbox" checked={d.enabled} disabled={busy}
              on:change={(e) => run(() => setPluginDataSync(d.pluginId, e.currentTarget.checked))} />
            {$t('sync.plugins.data.toggle', { plugin: d.name || d.pluginId })}
          </label>
          <div class="sync-meta">{$t('sync.plugins.data.files', { count: d.files })}</div>
          {#if d.waiting > 0}
            <p class="sync-hint">{$t('sync.plugins.data.waiting', { count: d.waiting, plugin: d.name || d.pluginId })}</p>
            <div class="sync-actions">
              <button type="button" class="secondary" disabled={busy}
                on:click={() => run(() => restartPluginForData(d.pluginId))}>{$t('sync.plugins.restart')}</button>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
    <p class="sync-hint">{$t('sync.plugins.data.hint')}</p>
  {/if}
  <p class="sync-error" role="alert">{error}</p>
</div>
{/if}

<InstallFlow
  bind:this={installFlow}
  on:installed={load}
  on:mismatch={(e) => (mismatch = e.detail)}
  on:error={(e) => showError(e.detail.message)}
/>
<!-- An install failure goes to the app-wide error dialog: the consent dialog stays open over this
     panel to allow a retry, and the panel's own error line sits in a section shown only with the
     advanced settings, so a refusal written there was seen by nobody. -->
