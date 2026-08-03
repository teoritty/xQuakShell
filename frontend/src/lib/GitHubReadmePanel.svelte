<script lang="ts">
  import { renderGitHubReadme } from './githubReadme';

  export let markdown = '';
  export let repositoryUrl = '';
  export let ref = 'main';

  $: html = renderGitHubReadme(markdown, repositoryUrl, ref);
</script>

{#if html}
  <div class="readme-markdown">{@html html}</div>
{:else}
  <p class="readme-empty">No README available.</p>
{/if}

<style>
  .readme-empty {
    margin: 0;
    font-size: 13px;
    color: var(--text-muted, #888);
  }

  .readme-markdown {
    font-size: 13px;
    line-height: 1.6;
    color: var(--text-primary);
    word-break: break-word;
  }

  .readme-markdown :global(h1),
  .readme-markdown :global(h2),
  .readme-markdown :global(h3),
  .readme-markdown :global(h4) {
    margin: 1.2em 0 0.6em;
    line-height: 1.3;
    color: var(--text-bright);
  }

  .readme-markdown :global(h1) { font-size: 1.35em; }
  .readme-markdown :global(h2) { font-size: 1.2em; }
  .readme-markdown :global(h3) { font-size: 1.08em; }

  .readme-markdown :global(p),
  .readme-markdown :global(ul),
  .readme-markdown :global(ol),
  .readme-markdown :global(blockquote),
  .readme-markdown :global(pre) {
    margin: 0.75em 0;
  }

  .readme-markdown :global(ul),
  .readme-markdown :global(ol) {
    padding-left: 1.4em;
  }

  .readme-markdown :global(blockquote) {
    padding-left: 12px;
    border-left: 3px solid var(--border-color, #333);
    color: var(--text-muted, #888);
  }

  .readme-markdown :global(code) {
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: 0.92em;
    padding: 0.1em 0.35em;
    border-radius: 4px;
    background: var(--bg-tertiary, rgba(255, 255, 255, 0.06));
  }

  .readme-markdown :global(pre) {
    overflow: auto;
    padding: 12px 14px;
    border-radius: 6px;
    border: 1px solid var(--border-color, #333);
    background: var(--bg-tertiary, rgba(0, 0, 0, 0.2));
  }

  .readme-markdown :global(pre code) {
    padding: 0;
    background: transparent;
  }

  .readme-markdown :global(table) {
    width: 100%;
    border-collapse: collapse;
    display: block;
    overflow-x: auto;
  }

  .readme-markdown :global(th),
  .readme-markdown :global(td) {
    border: 1px solid var(--border-color, #333);
    padding: 6px 10px;
    text-align: left;
  }

  .readme-markdown :global(a) {
    color: var(--accent, #6ea8fe);
    text-decoration: none;
  }

  .readme-markdown :global(a:hover) {
    text-decoration: underline;
  }

  .readme-markdown :global(img) {
    max-width: 100%;
    height: auto;
    border-radius: 6px;
    border: 1px solid var(--border-color, #333);
    background: var(--bg-tertiary, rgba(255, 255, 255, 0.04));
  }
</style>
