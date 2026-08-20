<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns the ping fields and the three transfer fields.
  export let draft: SettingsDraft;
</script>

<SettingsSection tab="network" section="ping" {view}>
  <div class="section">
    <h4>Connection ping</h4>
    <p class="section-desc">Check host reachability via TCP connect.</p>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.pingEnabled} />
      Enable automatic ping
    </label>
    <label class="setting-row">
      <span>Ping mode</span>
      <select bind:value={draft.pingMode} disabled={!draft.pingEnabled}>
        <option value="on_change">On connection settings change only</option>
        <option value="interval">Every N seconds</option>
      </select>
    </label>
    <label class="setting-row">
      <span>Ping interval (seconds)</span>
      <input
        type="number"
        bind:value={draft.pingIntervalSeconds}
        min="5"
        max="300"
        disabled={draft.pingMode !== 'interval' || !draft.pingEnabled}
      />
    </label>
    <label class="setting-row">
      <span>Max concurrent pings</span>
      <input
        type="number"
        bind:value={draft.maxConcurrentPings}
        min="1"
        max="64"
        disabled={!draft.pingEnabled}
      />
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="network" section="transfer" {view}>
  <div class="section">
    <h4>File transfer</h4>
    <label class="setting-row">
      <span>Speed limit (Kbps)</span>
      <input type="number" bind:value={draft.transferSpeedLimitKbps} min="0" placeholder="0 = unlimited" />
    </label>
    <label class="setting-row">
      <span>Connection timeout (seconds)</span>
      <input type="number" bind:value={draft.connectionTimeoutSeconds} min="5" max="300" />
    </label>
    <label class="setting-row">
      <span>Max concurrent transfers</span>
      <input type="number" bind:value={draft.maxConcurrentTransfers} min="1" max="16" />
    </label>
  </div>
</SettingsSection>
