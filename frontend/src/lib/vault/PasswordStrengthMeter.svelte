<script lang="ts">
  import type { StrengthResult } from './passwordStrength';

  /** `null` while the field is still empty: the meter shows, but stays neutral. */
  export let result: StrengthResult | null;

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

  $: filled = result ? SEGMENTS[result.label] : 0;
  $: tone = result ? result.label : 'idle';
  // Only the leading warning: the rest are usually restatements of the same
  // problem, and a list that grows and shrinks per keystroke moves the fields
  // underneath it.
  $: hint = result?.warnings[0] ?? '';
</script>

<div class="strength">
  <div class="bar" aria-hidden="true">
    {#each [1, 2, 3] as segment}
      <span class="segment {segment <= filled ? tone : ''}"></span>
    {/each}
  </div>

  <p class="verdict {tone}" aria-live="polite">
    {result ? `Password strength: ${LABELS[result.label]}` : 'Password strength'}
  </p>

  <!-- Reserved whether or not there is a hint, so the form never shifts. -->
  <p class="hint">{hint}</p>
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
    color: var(--text-secondary);
  }

  .verdict.weak { color: var(--danger); }
  .verdict.medium { color: var(--warning); }
  .verdict.strong { color: var(--success); }

  .hint {
    margin: 0;
    min-height: 15px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  @media (prefers-reduced-motion: reduce) {
    .segment { transition: none; }
  }
</style>
