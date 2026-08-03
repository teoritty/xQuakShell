<script lang="ts">
  import { Check, X } from 'lucide-svelte';
  import {
    MIN_MASTER_PASSWORD_LENGTH,
    RECOMMENDED_MASTER_PASSWORD_LENGTH,
    type PasswordChecklist,
  } from './passwordStrength';

  export let checklist: PasswordChecklist;

  // `required` marks the one rule that blocks submission; everything else is a
  // suggestion, and styling them alike would imply a policy that does not exist.
  $: rules = [
    { met: checklist.minLength, required: true, text: `At least ${MIN_MASTER_PASSWORD_LENGTH} characters` },
    { met: checklist.recommendedLength, required: false, text: `${RECOMMENDED_MASTER_PASSWORD_LENGTH} or more is much harder to guess` },
    { met: checklist.hasLower && checklist.hasUpper, required: false, text: 'Upper and lower case letters' },
    { met: checklist.hasDigit, required: false, text: 'A number' },
    { met: checklist.hasSymbol, required: false, text: 'A symbol' },
  ];
</script>

<ul class="requirements">
  {#each rules as rule}
    <li class:met={rule.met} class:blocking={rule.required && !rule.met}>
      {#if rule.met}
        <Check size={12} />
      {:else}
        <X size={12} />
      {/if}
      <span>{rule.text}</span>
    </li>
  {/each}
</ul>

<style>
  .requirements {
    width: 100%;
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  li {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  li.met {
    color: var(--success);
  }

  li.blocking {
    color: var(--text-primary);
  }
</style>
