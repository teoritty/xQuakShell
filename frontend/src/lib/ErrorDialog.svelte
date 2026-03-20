<script lang="ts">
  import Modal from './Modal.svelte';
  import { Copy, ExternalLink } from 'lucide-svelte';
  import { lastError, clearError } from '../stores/appState';

  let copied = false;

  $: show = $lastError !== null;

  function copyError() {
    if (!$lastError) return;
    const text = $lastError.details
      ? `${$lastError.message}\n\n${$lastError.details}`
      : $lastError.message;
    navigator.clipboard.writeText(text).then(() => {
      copied = true;
      setTimeout(() => { copied = false; }, 2000);
    });
  }

  const GITHUB_ISSUES_URL = 'https://github.com/xQuakShell/xQuakShell/issues/new';

  function openIssue() {
    if (!$lastError) return;
    const title = encodeURIComponent($lastError.message.slice(0, 100));
    const body = encodeURIComponent(
      `**Error:** ${$lastError.message}\n\n` +
      ($lastError.details ? `**Details:**\n\`\`\`\n${$lastError.details}\n\`\`\`\n\n` : '') +
      '---\n*Please describe what you were doing when this error occurred.*'
    );
    const url = `${GITHUB_ISSUES_URL}?title=${title}&body=${body}`;
    window.open(url, '_blank');
  }
</script>

{#if show && $lastError}
  <Modal title="Error" show={true} on:close={clearError}>
    <div class="error-body">
      <div class="error-message">{$lastError.message}</div>
      {#if $lastError.details}
        <pre class="error-details">{$lastError.details}</pre>
      {/if}
    </div>
    <div class="error-actions">
      <button class="secondary" on:click={copyError}>
        <Copy size={13} />
        {copied ? 'Copied' : 'Copy error'}
      </button>
      <button class="secondary" on:click={openIssue}>
        <ExternalLink size={13} />
        Open issue on GitHub
      </button>
      <button class="primary" on:click={clearError}>Close</button>
    </div>
  </Modal>
{/if}

<style>
  .error-body {
    margin-bottom: 16px;
  }

  .error-message {
    font-size: 13px;
    color: var(--danger);
    line-height: 1.5;
    margin-bottom: 8px;
  }

  .error-details {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-secondary);
    background: var(--bg-tertiary);
    border: 1px solid var(--border-color);
    border-radius: 2px;
    padding: 8px 10px;
    max-height: 200px;
    overflow-y: auto;
    white-space: pre-wrap;
    word-break: break-all;
  }

  .error-actions {
    display: flex;
    gap: 6px;
    justify-content: flex-end;
  }

  .error-actions button {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 4px 10px;
    font-size: 12px;
  }
</style>
