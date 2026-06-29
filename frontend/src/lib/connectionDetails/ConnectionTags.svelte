<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Plus, X } from 'lucide-svelte';
  import { isTagTooLong, isValidNewTag, tagColor, MAX_TAG_LENGTH } from './tags';

  export let tags: string[] = [];
  export let addingTag = false;
  export let newTagValue = '';

  $: tagTooLong = isTagTooLong(newTagValue);

  const dispatch = createEventDispatcher<{
    dirty: void;
    tagschange: string[];
    addingtagchange: boolean;
    newtagvaluechange: string;
  }>();

  function startAddTag() {
    dispatch('addingtagchange', true);
    dispatch('newtagvaluechange', '');
  }

  function confirmTag() {
    const t = newTagValue.trim();
    if (!isValidNewTag(t)) {
      dispatch('addingtagchange', false);
      dispatch('newtagvaluechange', '');
      return;
    }
    if (!tags.includes(t)) {
      dispatch('tagschange', [...tags, t]);
      dispatch('dirty');
    }
    dispatch('addingtagchange', false);
    dispatch('newtagvaluechange', '');
  }

  function cancelTag() {
    dispatch('addingtagchange', false);
    dispatch('newtagvaluechange', '');
  }

  function removeTag(tag: string) {
    dispatch('tagschange', tags.filter((x) => x !== tag));
    dispatch('dirty');
  }
</script>

<div class="field">
  <div class="section-header">
    <span class="field-label">Tags</span>
    <button class="ghost micro-btn" on:click={startAddTag}><Plus size={12} /> Tag</button>
  </div>
  <div class="tags-row">
    {#each tags as tag}
      <span class="tag-chip" style="background: {tagColor(tag)}">
        <span class="tag-label">{tag}</span>
        <button class="tag-remove" on:click={() => removeTag(tag)}><X size={9} /></button>
      </span>
    {/each}
    {#if addingTag}
      <div class="tag-input-wrap">
        <input
          class="tag-inline-input"
          class:invalid={tagTooLong}
          placeholder="tag name..."
          value={newTagValue}
          on:input={(e) => dispatch('newtagvaluechange', e.currentTarget.value)}
          on:keydown={(e) => {
            if (e.key === 'Enter') { e.preventDefault(); confirmTag(); }
            if (e.key === 'Escape') cancelTag();
          }}
          on:blur={confirmTag}
        />
        {#if tagTooLong}
          <span class="tag-error">Maximum {MAX_TAG_LENGTH} characters</span>
        {/if}
      </div>
    {/if}
    {#if tags.length === 0 && !addingTag}
      <span class="no-items">No tags</span>
    {/if}
  </div>
</div>

<style>
  .field { display: flex; flex-direction: column; gap: 2px; }
  .field-label { font-size: 11px; color: var(--text-secondary); font-weight: 500; }

  .section-header {
    display: flex; justify-content: space-between; align-items: center;
    margin-bottom: 4px;
  }

  .micro-btn {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 11px;
    padding: 1px 6px;
  }

  .no-items {
    font-size: 11px;
    color: var(--text-secondary);
    padding: 4px 0;
  }

  .tags-row {
    display: flex; flex-wrap: wrap; gap: 4px; align-items: center;
    min-height: 24px;
  }
  .tag-chip {
    display: inline-flex; align-items: center; gap: 3px;
    font-size: 10px; padding: 1px 6px; border-radius: 2px; color: #fff;
    max-width: 100%;
    min-width: 0;
  }
  .tag-label {
    min-width: 0;
    overflow-wrap: anywhere;
    word-break: break-word;
  }
  .tag-remove {
    background: none; border: none; color: rgba(255,255,255,0.7);
    cursor: pointer; padding: 0 1px; display: inline-flex; align-items: center;
    flex-shrink: 0;
  }
  .tag-remove:hover { color: #fff; }
  .tag-input-wrap {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    flex: 1 1 100%;
    max-width: 100%;
  }
  .tag-inline-input {
    width: 80px; font-size: 11px; padding: 2px 4px;
    background: var(--bg-input); border: 1px solid var(--border-focus);
    color: var(--text-primary); border-radius: 2px; outline: none;
  }
  .tag-inline-input.invalid {
    border-color: var(--danger);
  }
  .tag-error {
    font-size: 10px;
    color: var(--danger);
  }
</style>
