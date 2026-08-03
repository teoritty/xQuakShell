<script lang="ts">
  import { tick } from 'svelte';
  import { Eye, EyeOff } from 'lucide-svelte';

  export let value = '';
  export let placeholder = '';
  export let disabled = false;
  export let autofocus = false;
  export let ariaLabel: string;

  let revealed = false;
  let capsLock = false;
  let input: HTMLInputElement | undefined;

  // Deferred to the next microtask for the same reason as lib/focusSelect.ts:
  // WebView2 occasionally drops a focus() issued in the same tick as mount.
  function autofocusIf(node: HTMLInputElement, enabled: boolean) {
    if (enabled) queueMicrotask(() => node.focus());
  }

  // Caps Lock state is only readable from a keyboard event, so it appears on the
  // first keystroke and clears when the field loses focus.
  function trackModifiers(event: KeyboardEvent) {
    capsLock = event.getModifierState('CapsLock');
  }

  // Swapping the input type swaps the element, so focus has to be handed back
  // deliberately or the eye button would steal it mid-typing.
  async function toggleReveal() {
    revealed = !revealed;
    await tick();
    input?.focus();
  }
</script>

<div class="password-field">
  <!--
    Two inputs rather than one with a bound `type`: Svelte refuses a dynamic
    type attribute on an input that also uses two-way binding.
  -->
  {#if revealed}
    <input
      type="text"
      autocomplete="off"
      spellcheck="false"
      aria-label={ariaLabel}
      {placeholder}
      {disabled}
      bind:value
      bind:this={input}
      use:autofocusIf={autofocus}
      on:keydown={trackModifiers}
      on:keyup={trackModifiers}
      on:blur={() => (capsLock = false)}
    />
  {:else}
    <input
      type="password"
      autocomplete="off"
      spellcheck="false"
      aria-label={ariaLabel}
      {placeholder}
      {disabled}
      bind:value
      bind:this={input}
      use:autofocusIf={autofocus}
      on:keydown={trackModifiers}
      on:keyup={trackModifiers}
      on:blur={() => (capsLock = false)}
    />
  {/if}

  <button
    type="button"
    class="ghost reveal"
    aria-label={revealed ? 'Hide password' : 'Show password'}
    aria-pressed={revealed}
    tabindex="-1"
    {disabled}
    on:click={toggleReveal}
  >
    {#if revealed}
      <EyeOff size={15} />
    {:else}
      <Eye size={15} />
    {/if}
  </button>

  {#if capsLock}
    <span class="caps-lock" role="status">Caps Lock is on</span>
  {/if}
</div>

<style>
  .password-field {
    position: relative;
    width: 100%;
  }

  input {
    width: 100%;
    padding: 8px 34px 8px 12px;
    font-size: 14px;
  }

  .reveal {
    position: absolute;
    top: 0;
    right: 0;
    height: 30px;
    display: flex;
    align-items: center;
    padding: 0 9px;
    border-radius: 0 2px 2px 0;
  }

  .caps-lock {
    display: block;
    margin-top: 4px;
    font-size: 11px;
    color: var(--warning);
  }
</style>
