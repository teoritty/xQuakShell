<script lang="ts">
  import Modal from './Modal.svelte';
  import { Copy, ExternalLink } from 'lucide-svelte';
  import { lastError, clearError } from '../stores/appState';
  import { openNewIssue } from './projectLinks';
  import { t } from '../i18n/messages';

  let copied = false;
  /** What the issue button has to say for itself, when it has something to say. */
  let issueNoticeKey = '';

  $: show = $lastError !== null;
  // A new error clears the previous one's notice, so a message about a report that could not be
  // opened does not outlive the report it was about.
  $: if ($lastError) issueNoticeKey = '';

  function reportText(): string {
    if (!$lastError) return '';
    return $lastError.details
      ? `${$lastError.message}\n\n${$lastError.details}`
      : $lastError.message;
  }

  function copyError() {
    void copyReport().then((ok) => {
      if (!ok) return;
      copied = true;
      setTimeout(() => { copied = false; }, 2000);
    });
  }

  // Guarded because the clipboard is not always there to write to: it is unavailable over plain
  // http, and a denied permission rejects rather than throwing synchronously. The unguarded version
  // failed exactly like the button below it - nothing happened, and nothing said why.
  async function copyReport(): Promise<boolean> {
    const text = reportText();
    if (!text || !navigator.clipboard) return false;
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      return false;
    }
  }

  // The URL and its query belong to projectLinks, not here. A second copy of the address is what
  // sent this button to a non-existent owner once already, and a hand-built query string is what
  // mangled the stack trace it was collecting - a trace is exactly the payload full of the
  // characters (&, #, newlines) that only URLSearchParams encodes correctly.
  //
  // Three outcomes, and the two that are not "it worked" used to be indistinguishable from a dead
  // button. A report too long for the operating system's URL limit is the common one - a stack
  // trace is always too long - and it is the whole reason this button did nothing from here while
  // working from the About tab.
  async function openIssue() {
    if (!$lastError) return;
    const body =
      `**Error:** ${$lastError.message}\n\n` +
      ($lastError.details ? `**Details:**\n\`\`\`\n${$lastError.details}\n\`\`\`\n\n` : '') +
      '---\n*Please describe what you were doing when this error occurred.*';

    const { opened, complete } = openNewIssue($lastError.message.slice(0, 100), body);
    if (opened && complete) {
      issueNoticeKey = '';
      return;
    }
    // Either the form opened without the whole trace, or the browser never opened. Both leave the
    // user needing the text, so both put it where they can paste it.
    const onClipboard = await copyReport();
    if (!opened) {
      issueNoticeKey = onClipboard ? 'error.openIssueFailedCopied' : 'error.openIssueFailed';
      return;
    }
    issueNoticeKey = onClipboard ? 'error.openIssueTruncatedCopied' : 'error.openIssueTruncated';
  }
</script>

{#if show && $lastError}
  <Modal title={$t('error.title')} show={true} on:close={clearError}>
    <div class="error-body">
      <div class="error-message">{$lastError.message}</div>
      {#if $lastError.details}
        <pre class="error-details">{$lastError.details}</pre>
      {/if}
    </div>
    {#if issueNoticeKey}
      <p class="issue-notice">{$t(issueNoticeKey)}</p>
    {/if}
    <div class="error-actions">
      <button class="secondary" on:click={copyError}>
        <Copy size={13} />
        {copied ? $t('error.copied') : $t('error.copy')}
      </button>
      <button class="secondary" on:click={() => void openIssue()}>
        <ExternalLink size={13} />
        {$t('error.openIssue')}
      </button>
      <button class="primary" on:click={clearError}>{$t('common.close')}</button>
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

  .issue-notice {
    margin: 0 0 10px;
    font-size: 11px;
    line-height: 1.5;
    color: var(--warning);
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
