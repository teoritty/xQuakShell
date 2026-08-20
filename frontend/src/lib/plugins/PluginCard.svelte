<script lang="ts">
  // One plugin row, in both lists. It renders and dispatches; every decision about what an action
  // means belongs to the section that owns the data.
  //
  // The chip row is the point of the card. In an SSH client the question about a third-party
  // plugin is not "what version is it" but "what can it reach" - whether it is signed, whether the
  // OS is containing it, whether it can read the vault. Those sit on the face of the card rather
  // than behind a details dialog nobody opens.
  import { createEventDispatcher } from 'svelte';
  import { Info, Puzzle } from 'lucide-svelte';

  export let name: string;
  export let version = '';
  export let description = '';
  /** Where it came from, shown under the name: a repository URL or an author. */
  export let origin = '';
  /** Colour of the leading state dot. `none` omits it, for lists where state is not a property. */
  export let state: 'active' | 'attention' | 'idle' | 'none' = 'none';
  export let chips: { label: string; tone: 'good' | 'warn' | 'bad' | 'neutral' }[] = [];
  export let status = '';
  export let statusKind: 'installed' | 'not-installed' | 'warning' = 'not-installed';
  export let dimmed = false;
  export let showDetails = false;

  const dispatch = createEventDispatcher();
</script>

<div class="plugin-card" class:dimmed>
  <!-- One puzzle piece, not the plugin's initials. Initials were tried for identity and read as a
       contact list: two letters cut out of a name are not a name, and a plugin is the one thing in
       this app that already has a universally understood mark. -->
  <div class="card-icon" class:active={state === 'active'} class:attention={state === 'attention'}>
    <Puzzle size={17} />
  </div>

  <div class="card-main">
    <div class="card-head">
      <span class="card-name">{name}</span>
      {#if version}<span class="card-version">{version}</span>{/if}
    </div>
    {#if description}<div class="card-desc">{description}</div>{/if}
    {#if origin}<div class="card-origin">{origin}</div>{/if}
    {#if chips.length}
      <div class="card-chips">
        {#each chips as chip (chip.label)}
          <span class="chip chip-{chip.tone}">{chip.label}</span>
        {/each}
      </div>
    {/if}
  </div>

  <div class="card-side">
    {#if status}
      <span
        class="card-status"
        class:installed={statusKind === 'installed'}
        class:warning={statusKind === 'warning'}
      >
        {status}
      </span>
    {/if}
    <div class="card-actions">
      {#if showDetails}
        <button class="ghost icon-btn" title="Details" on:click={() => dispatch('details')}>
          <Info size={13} />
        </button>
      {/if}
      <slot name="actions" />
    </div>
  </div>
</div>

<style>
  .plugin-card {
    display: flex;
    align-items: flex-start;
    gap: 11px;
    padding: 11px 13px;
    border: 1px solid var(--border-color);
    border-radius: 5px;
    background: var(--bg-primary);
    transition: border-color 0.12s ease, background 0.12s ease;
  }

  .plugin-card:hover {
    border-color: var(--border-focus);
    background: var(--bg-tertiary);
  }

  .plugin-card.dimmed .card-icon,
  .plugin-card.dimmed .card-main {
    opacity: 0.62;
  }

  /* The state colour lives on the icon rather than on a separate dot: one object carrying identity
     and status reads faster than two competing for the same gutter. */
  .card-icon {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    margin-top: 1px;
    color: var(--text-secondary);
  }

  .card-icon.active {
    color: var(--success);
  }

  .card-icon.attention {
    color: var(--warning);
  }

  .card-main {
    min-width: 0;
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .card-head {
    display: flex;
    align-items: baseline;
    gap: 7px;
  }

  .card-name {
    font-weight: 600;
    font-size: 13px;
    color: var(--text-bright);
  }

  .card-version {
    font-family: var(--font-mono);
    font-size: 10.5px;
    color: var(--text-secondary);
  }

  .card-desc {
    font-size: 12px;
    color: var(--text-primary);
    overflow-wrap: anywhere;
  }

  .card-origin {
    font-size: 11px;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .card-chips {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin-top: 3px;
  }

  .chip {
    font-size: 10px;
    line-height: 1.6;
    padding: 0 6px;
    border-radius: 3px;
    border: 1px solid transparent;
    white-space: nowrap;
  }

  .chip-neutral {
    background: var(--bg-input);
    color: var(--text-secondary);
  }

  .chip-good {
    color: var(--success);
    border-color: rgba(90, 158, 94, 0.4);
  }

  .chip-warn {
    color: var(--warning);
    border-color: rgba(196, 144, 64, 0.45);
  }

  .chip-bad {
    color: var(--danger);
    border-color: rgba(197, 80, 80, 0.45);
  }

  .card-side {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 6px;
    flex-shrink: 0;
  }

  .card-status {
    font-size: 11px;
    color: var(--text-secondary);
    white-space: nowrap;
  }

  .card-status.installed {
    color: var(--success);
  }

  .card-status.warning {
    color: var(--warning);
  }

  .card-actions {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .icon-btn {
    display: inline-flex;
    align-items: center;
    padding: 3px;
  }
</style>
