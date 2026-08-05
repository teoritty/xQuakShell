<script lang="ts">
  import type { FieldGroup, FieldDef } from '../../actions/protocolActions';
  // Layout is shared with the plugin dialog and the node details panel (ADR-015): the markup here
  // is mostly about vault-stored secrets, which those must not have, but how fields are ordered,
  // hidden and packed into rows must not diverge between them.
  import { groupFieldsIntoRows, isFieldVisible } from '../fields/layout';
  import { createEventDispatcher } from 'svelte';
  import './connectionDetailsShared.css';

  export let groups: FieldGroup[] = [];
  export let values: Record<string, unknown> = {};
  export let errors: Record<string, string> = {};
  export let readonly = false;
  // Plugin field ids whose secret is already stored in the vault (value masked to "").
  export let storedSecretFields: string[] = [];
  $: storedSecretSet = new Set(storedSecretFields);

  // Classic "already saved" password UX. The real secret never leaves the host, so a stored secret
  // arrives with an empty value; we render a fixed dot mask so the field reads as a filled password.
  // Focusing it reveals an empty box to type a replacement; blurring without typing restores the
  // mask and leaves the field UNTOUCHED, so the save payload omits it and the backend keeps the
  // stored secret (a fake value is never submitted). The mask is display-only, never in the model.
  const STORED_SECRET_MASK = '••••••••••';
  let secretFocused: Record<string, boolean> = {};
  let secretEdited: Record<string, boolean> = {};

  function isStoredSecret(field: FieldDef): boolean {
    return field.type === 'password' && storedSecretSet.has(field.id);
  }
  function passwordValue(field: FieldDef): string {
    if (isStoredSecret(field) && !secretEdited[field.id] && !secretFocused[field.id]) {
      return STORED_SECRET_MASK;
    }
    return String(values[field.id] ?? '');
  }
  function onSecretFocus(field: FieldDef) {
    if (isStoredSecret(field)) secretFocused = { ...secretFocused, [field.id]: true };
  }
  function onSecretBlur(field: FieldDef) {
    if (isStoredSecret(field)) secretFocused = { ...secretFocused, [field.id]: false };
  }
  function onPasswordInput(field: FieldDef, value: string) {
    if (isStoredSecret(field)) secretEdited = { ...secretEdited, [field.id]: true };
    handleInput(field, value);
  }

  const dispatch = createEventDispatcher<{ fieldchange: { fieldId: string; value: unknown } }>();

  type FieldRow = { kind: 'row'; fields: FieldDef[] } | { kind: 'single'; field: FieldDef };

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
      rows: groupFieldsIntoRows(
        [...g.fields].sort((a, b) => a.order - b.order),
        values,
      ),
    }));


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

{#each sortedGroups as group}
  <div class="connection-detail-field">
    {#if group.label}
      <div class="connection-detail-section-header">
        <span class="connection-detail-field-label">{group.label}</span>
      </div>
    {/if}

    {#each group.rows as row}
      {#if row.kind === 'row'}
        <div class="connection-detail-field-row">
          {#each row.fields as field (field.id)}
            <label class="connection-detail-field">
              <span class="connection-detail-field-label">
                {field.label}
                {#if field.required}<span class="connection-detail-required">*</span>{/if}
              </span>

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
                  value={passwordValue(field)}
                  placeholder={field.placeholder ?? ''}
                  disabled={readonly}
                  on:focus={() => onSecretFocus(field)}
                  on:blur={() => onSecretBlur(field)}
                  on:input={(e) => onPasswordInput(field, e.currentTarget.value)}
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
              {:else if field.type === 'textarea'}
                <textarea
                  id="field-{field.id}"
                  value={String(values[field.id] ?? field.default ?? '')}
                  placeholder={field.placeholder ?? ''}
                  disabled={readonly}
                  on:input={(e) => handleInput(field, e.currentTarget.value)}
                ></textarea>
              {/if}

              {#if field.description && field.type !== 'checkbox'}
                <span class="connection-detail-field-hint">{field.description}</span>
              {/if}

              {#if errors[field.id]}
                <span class="connection-detail-field-error">{errors[field.id]}</span>
              {/if}
            </label>
          {/each}
        </div>
      {:else}
        {@const field = row.field}
        {#if field.type === 'checkbox'}
          <div class="connection-detail-field">
            <label class="connection-detail-checkbox">
              <input
                id="field-{field.id}"
                type="checkbox"
                checked={checkboxValue(field)}
                disabled={readonly}
                on:change={(e) => handleInput(field, serializeCheckbox(e.currentTarget.checked))}
              />
              <span>
                {field.label}
                {#if field.required}<span class="connection-detail-required">*</span>{/if}
              </span>
            </label>
            {#if field.description}
              <span class="connection-detail-field-hint">{field.description}</span>
            {/if}
            {#if errors[field.id]}
              <span class="connection-detail-field-error">{errors[field.id]}</span>
            {/if}
          </div>
        {:else}
          <label class="connection-detail-field">
            <span class="connection-detail-field-label">
              {field.label}
              {#if field.required}<span class="connection-detail-required">*</span>{/if}
            </span>

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
                value={passwordValue(field)}
                placeholder={field.placeholder ?? ''}
                disabled={readonly}
                on:focus={() => onSecretFocus(field)}
                on:blur={() => onSecretBlur(field)}
                on:input={(e) => onPasswordInput(field, e.currentTarget.value)}
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
              <span class="connection-detail-field-hint">{field.description}</span>
            {/if}

            {#if errors[field.id]}
              <span class="connection-detail-field-error">{errors[field.id]}</span>
            {/if}
          </label>
        {/if}
      {/if}
    {/each}
  </div>
{/each}
