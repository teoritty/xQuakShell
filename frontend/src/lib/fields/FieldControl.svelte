<script lang="ts">
  // One field of a dialog or a node details panel (ADR-015).
  //
  // Deliberately not the connection editor's renderer: that component is mostly about vault-stored
  // secrets, which a dialog forbids outright, so reusing it would carry a concern into a place that
  // must not have it. What the two DO share — ordering, visibility, row packing and validation —
  // lives in ./layout.ts and ./validate.ts and is shared.
  import type { ValidatableField } from './validate';
  import {
    newRow,
    rowProblems,
    rowsFromValue,
    rowsMatchValue,
    valueFromRows,
    type KeyValueRow,
  } from './keyValueRows';
  import { Plus, Trash2, Copy } from 'lucide-svelte';

  export let field: ValidatableField & { placeholder?: string; description?: string };
  export let value: string = '';
  export let error: string = '';
  export let readonly: boolean = false;
  export let onChange: (value: string) => void;

  /**
   * Rows are the editor's state, and the JSON object is what they render down to.
   *
   * Deriving the rows from `value` on every keystroke is what used to delete a row the moment its
   * name was cleared for retyping, and merge two rows that briefly shared a name. They are re-read
   * only when the value changes underneath the editor — a different field, a snapshot pushed by
   * the plugin — never as an echo of what this component just emitted.
   */
  let rows: KeyValueRow[] = [];
  let rowsFieldID = '';

  $: if (field.type === 'keyValue' && (field.id !== rowsFieldID || !rowsMatchValue(rows, value))) {
    rowsFieldID = field.id;
    rows = rowsFromValue(value);
  }
  $: problems = field.type === 'keyValue' ? rowProblems(rows) : new Map<number, string>();

  function commitRows(next: KeyValueRow[]) {
    rows = next;
    onChange(valueFromRows(next));
  }

  function updateRow(id: number, patch: Partial<KeyValueRow>) {
    commitRows(rows.map((row) => (row.id === id ? { ...row, ...patch } : row)));
  }

  function addRow() {
    commitRows([...rows, newRow()]);
  }

  function removeRow(id: number) {
    commitRows(rows.filter((row) => row.id !== id));
  }

  async function copyCode() {
    try {
      await navigator.clipboard.writeText(value);
    } catch (e) {
      console.debug('[field copy]', e);
    }
  }
</script>

<div class="field" class:has-error={!!error}>
  <label for={`field-${field.id}`}>
    {field.label ?? field.id}
    {#if field.required}<span class="required">*</span>{/if}
  </label>

  {#if field.type === 'checkbox'}
    <input
      id={`field-${field.id}`}
      type="checkbox"
      checked={value === 'true'}
      disabled={readonly}
      on:change={(e) => onChange(e.currentTarget.checked ? 'true' : 'false')}
    />
  {:else if field.type === 'select'}
    <select
      id={`field-${field.id}`}
      {value}
      disabled={readonly}
      on:change={(e) => onChange(e.currentTarget.value)}
    >
      {#each field.options ?? [] as opt (opt.value)}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
  {:else if field.type === 'textarea'}
    <textarea
      id={`field-${field.id}`}
      rows="4"
      {value}
      placeholder={field.placeholder ?? ''}
      disabled={readonly}
      on:input={(e) => onChange(e.currentTarget.value)}
    ></textarea>
  {:else if field.type === 'code'}
    <div class="code-block">
      <button class="ghost copy" title="Copy" on:click={copyCode}><Copy size={12} /></button>
      <pre>{value}</pre>
    </div>
  {:else if field.type === 'keyValue'}
    <div class="pairs">
      <!-- Keyed by the row's own id, not its position: an id survives a rename, so the input the
           user is typing in keeps its focus and its cursor. -->
      {#each rows as row (row.id)}
        <div class="pair">
          <input
            class="pair-key"
            class:invalid={problems.has(row.id)}
            value={row.key}
            placeholder="name"
            disabled={readonly}
            title={problems.get(row.id) ?? ''}
            on:input={(e) => updateRow(row.id, { key: e.currentTarget.value })}
          />
          <input
            class="pair-value"
            value={row.value}
            placeholder="value"
            disabled={readonly}
            on:input={(e) => updateRow(row.id, { value: e.currentTarget.value })}
          />
          {#if !readonly}
            <button class="ghost" title="Remove" on:click={() => removeRow(row.id)}>
              <Trash2 size={12} />
            </button>
          {/if}
        </div>
        {#if problems.has(row.id)}
          <p class="error">{problems.get(row.id)}</p>
        {/if}
      {/each}
      {#if !readonly}
        <button class="ghost add" on:click={addRow}><Plus size={12} /> Add entry</button>
      {/if}
    </div>
  {:else}
    <input
      id={`field-${field.id}`}
      type={field.type === 'number' ? 'number' : 'text'}
      {value}
      placeholder={field.placeholder ?? ''}
      disabled={readonly}
      on:input={(e) => onChange(e.currentTarget.value)}
    />
  {/if}

  {#if field.description}
    <p class="description">{field.description}</p>
  {/if}
  {#if error}
    <p class="error">{error}</p>
  {/if}
</div>

<style>
  .field {
    display: flex;
    flex-direction: column;
    gap: 3px;
    flex: 1;
    min-width: 0;
  }

  label {
    font-size: 11px;
    color: var(--text-secondary);
  }

  .required {
    color: var(--danger);
  }

  input,
  select,
  textarea {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    color: var(--text-primary);
    border-radius: 3px;
    padding: 4px 6px;
    font-size: 12px;
    width: 100%;
  }

  input[type='checkbox'] {
    width: auto;
  }

  .has-error input,
  .has-error select,
  .has-error textarea {
    border-color: var(--danger);
  }

  .description {
    font-size: 11px;
    color: var(--text-secondary);
    margin: 0;
  }

  .error {
    font-size: 11px;
    color: var(--danger);
    margin: 0;
  }

  .code-block {
    position: relative;
    max-height: 320px;
    overflow: auto;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 3px;
  }

  .code-block pre {
    margin: 0;
    padding: 6px 8px;
    font-family: var(--font-mono, monospace);
    font-size: 12px;
    white-space: pre;
  }

  .copy {
    position: sticky;
    float: right;
    top: 4px;
    right: 4px;
    z-index: 1;
  }

  .pairs {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .pair {
    display: flex;
    gap: 4px;
    align-items: center;
  }

  .pair-key {
    flex: 0 0 35%;
  }

  .pair-key.invalid {
    border-color: var(--danger);
  }

  .add {
    align-self: flex-start;
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
  }
</style>
