<script lang="ts">
  // Presentational chrome shared by the create and unlock screens. Owns no
  // behaviour: the forms decide what goes inside it.
  export let title: string;
  export let subtitle = '';
  export let error = '';
  /** The create screen carries a checklist and a meter, so it needs more room. */
  export let wide = false;
</script>

<div class="vault-card" class:wide>
  <div class="vault-icon"><slot name="icon" /></div>
  <h2>{title}</h2>
  {#if subtitle}
    <p class="vault-subtitle">{subtitle}</p>
  {/if}

  {#if error}
    <div class="vault-error" role="alert">{error}</div>
  {/if}

  <slot />
</div>

<style>
  .vault-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 40px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 8px;
    /* Fixed rather than min-width: the long copy on the create screen would
       otherwise stretch the card across the whole window. */
    width: 360px;
    max-width: 100%;
    box-shadow: 0 4px 24px rgba(0, 0, 0, 0.3);
  }

  .vault-card.wide {
    width: 420px;
  }

  .vault-icon {
    color: var(--text-secondary);
    line-height: 0;
  }

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--text-bright);
    margin: 0;
  }

  .vault-subtitle {
    font-size: 13px;
    color: var(--text-secondary);
    margin: 0;
    text-align: center;
  }

  .vault-error {
    padding: 8px 12px;
    background: rgba(197, 80, 80, 0.15);
    border: 1px solid var(--danger);
    border-radius: 4px;
    color: var(--danger);
    font-size: 12px;
    width: 100%;
    text-align: center;
  }
</style>
