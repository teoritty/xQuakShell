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

  // The type is driven imperatively, and the value flows through an explicit
  // input handler rather than bind:value, because Svelte rejects a dynamic
  // `type` on a two-way-bound input. Doing it this way keeps ONE input element:
  // duplicating it per state would mean keeping two copies of every attribute
  // in sync, and would tear down and rebuild the element on every toggle,
  // losing focus and caret position with it.
  $: if (input) input.type = revealed ? 'text' : 'password';

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

  // Clicking the eye moves focus to the button and drops the caret, so both are
  // captured and put back once the reactive type change has landed. Without
  // this, revealing mid-word sends the next keystroke to the start of the field.
  async function toggleReveal() {
    const start = input?.selectionStart ?? null;
    const end = input?.selectionEnd ?? null;

    revealed = !revealed;
    await tick();

    input?.focus();
    if (input && start !== null && end !== null) {
      input.setSelectionRange(start, end);
    }
  }
</script>

<div class="password-field">
  <!--
    The row exists so the reveal button can stretch to exactly the input's
    height. The Caps Lock note sits outside it, clear of that stretch.
  -->
  <div class="input-row">
    <input
      type="password"
      autocomplete="off"
      spellcheck="false"
      aria-label={ariaLabel}
      {placeholder}
      {disabled}
      {value}
      bind:this={input}
      use:autofocusIf={autofocus}
      on:input={(e) => (value = e.currentTarget.value)}
      on:keydown={trackModifiers}
      on:keyup={trackModifiers}
      on:blur={() => (capsLock = false)}
    />

    <button
      type="button"
      class="reveal"
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
  </div>

  {#if capsLock}
    <span class="caps-lock" role="status">Caps Lock is on</span>
  {/if}
</div>

<style>
  .password-field {
    width: 100%;
  }

  .input-row {
    position: relative;
    display: flex;
  }

  input {
    width: 100%;
    padding: 8px 34px 8px 12px;
    font-size: 14px;
  }

  /*
    WebView2 and Edge draw their own reveal control inside password inputs. Left
    alone it sits beside ours and the field shows two eyes that disagree about
    what they are toggling. The WebKit variants are suppressed for the same
    reason.
  */
  input::-ms-reveal,
  input::-ms-clear,
  input::-webkit-credentials-auto-fill-button,
  input::-webkit-strong-password-auto-fill-button {
    display: none !important;
    visibility: hidden;
    pointer-events: none;
  }

  /*
    Stretched rather than given a height: the input's height comes from its font
    size, padding and --ui-scale, so any fixed number here would drift out of
    alignment the moment one of those changes. Inset by the input's 1px border
    so it sits inside the field and never paints over the focus outline.

    The icon is the whole control: no box, no fill, in any state. Every global
    button style that would draw one is turned off here rather than inherited
    and fought with later.
  */
  .reveal {
    position: absolute;
    top: 1px;
    bottom: 1px;
    right: 1px;
    display: flex;
    align-items: center;
    padding: 0 9px;
    background: none;
    border: none;
    border-radius: 0;
    color: var(--text-secondary);
    transition: color 0.12s;
  }

  .reveal:hover,
  .reveal:focus,
  .reveal:active,
  .reveal:disabled {
    background: none;
    border: none;
  }

  .reveal:hover {
    color: var(--text-bright);
  }

  @media (prefers-reduced-motion: reduce) {
    .reveal { transition: none; }
  }

  .caps-lock {
    display: block;
    margin-top: 4px;
    font-size: 11px;
    color: var(--warning);
  }
</style>
