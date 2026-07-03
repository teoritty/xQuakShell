<script lang="ts">
  import type { FieldGroup, FieldDef } from '../../stores/api';
  import { createEventDispatcher } from 'svelte';

  export let groups: FieldGroup[] = [];
  export let values: Record<string, unknown> = {};
  export let errors: Record<string, string> = {};
  export let readonly = false;

  const dispatch = createEventDispatcher<{ fieldchange: { fieldId: string; value: unknown } }>();

  $: compiledPatterns = (() => {
    const result: Record<string, RegExp> = {};
    for (const group of groups) {
      for (const field of group.fields) {
        if (field.validation?.pattern) {
          try {
            result[field.id] = new RegExp(field.validation.pattern);
          } catch (e) {
            console.error(`Invalid pattern for field ${field.id}:`, e);
          }
        }
      }
    }
    return result;
  })();

  $: sortedGroups = [...groups]
    .sort((a, b) => a.order - b.order)
    .map((g) => ({
      ...g,
      fields: [...g.fields].sort((a, b) => a.order - b.order),
    }));

  function isVisible(field: FieldDef): boolean {
    if (!field.dependsOn) return true;
    return !!values[field.dependsOn];
  }

  function validateField(field: FieldDef): string {
    const val = values[field.id];
    if (field.required && (val === undefined || val === null || val === '')) {
      return `${field.label} is required`;
    }
    if (field.validation) {
      const v = field.validation;
      if (typeof val === 'string') {
        if (v.minLength && val.length < v.minLength) return `Min length: ${v.minLength}`;
        if (v.maxLength && val.length > v.maxLength) return `Max length: ${v.maxLength}`;
        if (v.pattern && compiledPatterns[field.id] && !compiledPatterns[field.id].test(val)) {
          return 'Invalid format';
        }
      }
      if (typeof val === 'number') {
        if (v.min !== undefined && val < v.min) return `Min: ${v.min}`;
        if (v.max !== undefined && val > v.max) return `Max: ${v.max}`;
      }
    }
    return '';
  }

  function handleInput(field: FieldDef, value: unknown) {
    values = { ...values, [field.id]: value };
    errors = { ...errors, [field.id]: validateField(field) };
    dispatch('fieldchange', { fieldId: field.id, value });
  }

  function widthClass(w?: string): string {
    switch (w) {
      case 'half':
        return 'field-half';
      case 'third':
        return 'field-third';
      default:
        return 'field-full';
    }
  }

  function checkboxValue(field: FieldDef): boolean {
    const val = values[field.id];
    if (typeof val === 'boolean') return val;
    if (val === 'true') return true;
    if (val === 'false') return false;
    return Boolean(field.default ?? false);
  }

  function serializeCheckbox(checked: boolean): string {
    return checked ? 'true' : 'false';
  }
</script>

<div class="plugin-fields">
  {#each sortedGroups as group}
    <fieldset class="field-group">
      {#if group.label}
        <legend>{group.label}</legend>
      {/if}

      <div class="fields-row">
        {#each group.fields as field}
          {#if isVisible(field)}
            <div class="field-wrapper {widthClass(field.width)}">
              <label for="field-{field.id}">
                {field.label}
                {#if field.required}<span class="required">*</span>{/if}
              </label>

              {#if field.type === 'text'}
                <input
                  id="field-{field.id}"
                  type="text"
                  value={String(values[field.id] ?? field.default ?? '')}
                  placeholder={field.placeholder ?? ''}
                  disabled={readonly}
                  on:input={(e) => handleInput(field, e.currentTarget.value)}
                />
              {:else if field.type === 'password'}
                <input
                  id="field-{field.id}"
                  type="password"
                  value={String(values[field.id] ?? '')}
                  placeholder={field.placeholder ?? ''}
                  disabled={readonly}
                  on:input={(e) => handleInput(field, e.currentTarget.value)}
                />
              {:else if field.type === 'number'}
                <input
                  id="field-{field.id}"
                  type="number"
                  value={values[field.id] ?? field.default ?? ''}
                  disabled={readonly}
                  on:input={(e) => handleInput(field, Number(e.currentTarget.value))}
                />
              {:else if field.type === 'select'}
                <select
                  id="field-{field.id}"
                  value={String(values[field.id] ?? field.default ?? '')}
                  disabled={readonly}
                  on:change={(e) => handleInput(field, e.currentTarget.value)}
                >
                  {#each field.options ?? [] as opt}
                    <option value={opt.value}>{opt.label}</option>
                  {/each}
                </select>
              {:else if field.type === 'checkbox'}
                <input
                  id="field-{field.id}"
                  type="checkbox"
                  checked={checkboxValue(field)}
                  disabled={readonly}
                  on:change={(e) => handleInput(field, serializeCheckbox(e.currentTarget.checked))}
                />
              {:else if field.type === 'textarea'}
                <textarea
                  id="field-{field.id}"
                  value={String(values[field.id] ?? field.default ?? '')}
                  placeholder={field.placeholder ?? ''}
                  disabled={readonly}
                  on:input={(e) => handleInput(field, e.currentTarget.value)}
                ></textarea>
              {/if}

              {#if field.description}
                <small class="field-description">{field.description}</small>
              {/if}

              {#if errors[field.id]}
                <span class="field-error">{errors[field.id]}</span>
              {/if}
            </div>
          {/if}
        {/each}
      </div>
    </fieldset>
  {/each}
</div>

<style>
  .plugin-fields {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .field-group {
    border: 1px solid var(--border-color);
    border-radius: 8px;
    padding: 16px;
  }

  .fields-row {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
  }

  .field-wrapper {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .field-full {
    width: 100%;
  }

  .field-half {
    width: calc(50% - 6px);
  }

  .field-third {
    width: calc(33.33% - 8px);
  }

  .required {
    color: var(--danger);
  }

  .field-error {
    color: var(--danger);
    font-size: 0.85em;
  }

  .field-description {
    color: var(--text-secondary);
    font-size: 0.85em;
  }
</style>
