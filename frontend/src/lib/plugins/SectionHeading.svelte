<script lang="ts">
  // Every section opens the same way: what this page is, and one line saying what it is for.
  //
  // It exists because four sections repeating the same header markup is where they drift apart,
  // and because the subtitle is load-bearing rather than decorative - each section of this screen
  // does something a user can get wrong, and the line is where that is said.
  export let title: string;
  export let subtitle = '';
  /** Free-form, e.g. "3 of 11" while searching. Empty renders nothing. */
  export let count = '';
</script>

<header class="section-heading">
  <div class="heading-row">
    <h4>{title}</h4>
    {#if count}<span class="heading-count">{count}</span>{/if}
    <slot name="action" />
  </div>
  {#if subtitle}<p class="heading-sub">{subtitle}</p>{/if}
</header>

<style>
  .section-heading {
    margin-bottom: 14px;
  }

  .heading-row {
    display: flex;
    align-items: center;
    gap: 9px;
  }

  h4 {
    margin: 0;
    font-size: 14px;
    font-weight: 600;
    color: var(--text-bright);
  }

  .heading-count {
    font-family: var(--font-mono);
    font-size: 10.5px;
    padding: 1px 6px;
    border-radius: 3px;
    background: var(--bg-input);
    color: var(--text-secondary);
  }

  .heading-sub {
    margin: 3px 0 0;
    max-width: 62ch;
    font-size: 11.5px;
    line-height: 1.5;
    color: var(--text-secondary);
  }
</style>
