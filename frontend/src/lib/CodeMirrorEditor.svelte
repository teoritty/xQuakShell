<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { EditorView, basicSetup } from 'codemirror';
  import { StreamLanguage } from '@codemirror/language';
  import { shell } from '@codemirror/legacy-modes/mode/shell';
  import { EditorState } from '@codemirror/state';

  export let value = '';
  export let minHeight = '200px';

  let container: HTMLDivElement;
  let view: EditorView | null = null;

  $: if (view) {
    const docStr = view.state.doc.toString();
    if (value !== docStr) {
      view.dispatch({
        changes: { from: 0, to: docStr.length, insert: value },
      });
    }
  }

  onMount(() => {
    const shellLanguage = StreamLanguage.define(shell);
    const state = EditorState.create({
      doc: value,
      extensions: [
        basicSetup,
        shellLanguage,
        EditorView.updateListener.of((v) => {
          if (v.docChanged) {
            value = v.state.doc.toString();
          }
        }),
      ],
    });
    view = new EditorView({
      state,
      parent: container,
    });
  });

  onDestroy(() => {
    view?.destroy();
    view = null;
  });

  export function getValue(): string {
    return view?.state.doc.toString() ?? value;
  }

  export function setValue(v: string) {
    value = v;
    if (view) {
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: v },
      });
    }
  }
</script>

<div
  class="cm-wrapper"
  bind:this={container}
  style="min-height: {minHeight}"
  role="textbox"
  aria-multiline="true"
></div>

<style>
  .cm-wrapper {
    flex: 1;
    min-height: 200px;
    overflow: auto;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    background: var(--bg-tertiary);
  }

  .cm-wrapper :global(.cm-editor) {
    min-height: 100%;
    font-size: 12px;
  }

  .cm-wrapper :global(.cm-scroller) {
    font-family: var(--font-mono, monospace);
  }

  .cm-wrapper :global(.cm-content) {
    padding: 8px 0;
  }

  .cm-wrapper :global(.cm-gutters) {
    background: var(--bg-secondary);
    border: none;
  }

  .cm-wrapper :global(.cm-activeLineGutter) {
    background: var(--bg-hover);
  }
</style>
