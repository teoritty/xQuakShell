<script lang="ts">
  // One section of the settings dialog, with the two decisions every section shares: whether it is
  // visible at all, and whether it needs the tab name above it.
  //
  // Both were spelled out inline at each of the twenty section sites, which meant the search
  // behaviour of the dialog was defined twenty times over and a section added without the second
  // half rendered under whichever tab name happened to precede it.
  import {
    SETTINGS_TAB_LABELS,
    shouldShowSettingsSection,
    shouldShowSectionTabLabel,
    type SettingsSearchViewState,
    type SettingsTabId,
  } from '../settingsSearch';

  export let tab: SettingsTabId;
  export let section: string;
  export let view: SettingsSearchViewState;

  $: visible = view.isSearching
    ? shouldShowSettingsSection(tab, section, view)
    : view.activeTab === tab;
  $: labelled = shouldShowSectionTabLabel(tab, section, view);
</script>

{#if visible}
  {#if labelled}
    <div class="section-tab-label">{SETTINGS_TAB_LABELS[tab]}</div>
  {/if}
  <slot />
{/if}
