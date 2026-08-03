<script lang="ts">
  import type { StrengthResult } from './passwordStrength';

  export let result: StrengthResult;

  const LABELS: Record<StrengthResult['label'], string> = {
    weak: 'Weak',
    medium: 'Medium',
    strong: 'Strong',
  };

  const SEGMENTS: Record<StrengthResult['label'], number> = {
    weak: 1,
    medium: 2,
    strong: 3,
  };

  $: filled = SEGMENTS[result.label];
</script>

<div class="strength">
  <div class="bar" aria-hidden="true">
    {#each [1, 2, 3] as segment}
      <span class="segment {segment <= filled ? result.label : ''}"></span>
    {/each}
  </div>

  <p class="verdict {result.label}" aria-live="polite">
    Password strength: {LABELS[result.label]}
  </p>

  {#if result.warnings.length > 0}
    <ul class="warnings">
      {#each result.warnings as warning}
        <li>{warning}</li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .strength {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  .bar {
    display: flex;
    gap: 4px;
  }

  .segment {
    flex: 1;
    height: 4px;
    border-radius: 2px;
    background: var(--bg-input);
    transition: background 0.15s;
  }

  .segment.weak { background: var(--danger); }
  .segment.medium { background: var(--warning); }
  .segment.strong { background: var(--success); }

  .verdict {
    margin: 0;
    font-size: 11px;
  }

  .verdict.weak { color: var(--danger); }
  .verdict.medium { color: var(--warning); }
  .verdict.strong { color: var(--success); }

  .warnings {
    margin: 0;
    padding-left: 16px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  @media (prefers-reduced-motion: reduce) {
    .segment { transition: none; }
  }
</style>
