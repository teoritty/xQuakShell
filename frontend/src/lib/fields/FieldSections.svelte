<script lang="ts">
  // Renders a declarative field schema: sections, ordering, visibility, row packing.
  //
  // Shared by the plugin dialog and the discovery node details panel. Both hand it the same shape
  // and get back the same layout, which is the point — a form that looks different depending on
  // where it was opened is a form the user has to re-learn.
  import FieldControl from './FieldControl.svelte';
  import { groupFieldsIntoRows, sortByOrder, type LayoutField } from './layout';
  import type { ValidatableField } from './validate';

  type Field = ValidatableField &
    LayoutField & { placeholder?: string; description?: string };

  export let sections: { id: string; label: string; order?: number; fields: Field[] }[] = [];
  export let values: Record<string, string> = {};
  export let errors: Record<string, string> = {};
  export let readonly: boolean = false;
  export let onChange: (fieldId: string, value: string) => void;

  $: ordered = [...sections]
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
    .map((section) => ({
      ...section,
      rows: groupFieldsIntoRows(sortByOrder(section.fields), values),
    }));
</script>

{#each ordered as section (section.id)}
  <section class="field-section">
    {#if section.label}
      <h4>{section.label}</h4>
    {/if}
    {#each section.rows as row, i (i)}
      {#if row.kind === 'single'}
        <FieldControl
          field={row.field}
          value={values[row.field.id] ?? ''}
          error={errors[row.field.id] ?? ''}
          {readonly}
          onChange={(v) => onChange(row.field.id, v)}
        />
      {:else}
        <div class="field-row">
          {#each row.fields as field (field.id)}
            <FieldControl
              {field}
              value={values[field.id] ?? ''}
              error={errors[field.id] ?? ''}
              {readonly}
              onChange={(v) => onChange(field.id, v)}
            />
          {/each}
        </div>
      {/if}
    {/each}
  </section>
{/each}

<style>
  .field-section {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-bottom: 12px;
  }

  h4 {
    margin: 0;
    font-size: 12px;
    font-weight: 600;
    color: var(--text-bright);
  }

  .field-row {
    display: flex;
    gap: 8px;
  }
</style>
