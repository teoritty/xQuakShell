<script lang="ts">
  import { onDestroy } from 'svelte';
  import type { Session } from '../stores/appState';
  import { reportEmbedActivity, reportEmbedViewport } from '../stores/api';

  export let session: Session;
  export let active: boolean = false;

  let iframeEl: HTMLIFrameElement | null = null;
  let hostPort: MessagePort | null = null;
  let refitRaf = 0;
  let resizeObserver: ResizeObserver | null = null;
  let containerEl: HTMLDivElement | null = null;
  let lastViewport = '';

  function iframeSrc(url: string): string {
    if (!url) return url;
    try {
      const resolved = new URL(url, window.location.href);
      resolved.searchParams.set('hostOrigin', window.location.origin);
      return resolved.toString();
    } catch {
      return url;
    }
  }

  function sandboxAttr(attrs: string[] | undefined): string {
    return (attrs && attrs.length > 0 ? attrs : ['allow-scripts', 'allow-same-origin']).join(' ');
  }

  function postToIframe(message: Record<string, unknown>) {
    hostPort?.postMessage({ source: 'xquakshell-host', ...message });
  }

  function setupIframeChannel() {
    const target = iframeEl?.contentWindow;
    if (!target || hostPort) return;

    const channel = new MessageChannel();
    hostPort = channel.port1;
    target.postMessage({ source: 'xquakshell-host-init', sessionId: session.sessionId }, '*', [channel.port2]);
  }

  function measureViewport(): { widthPx: number; heightPx: number; devicePixelRatio: number } | null {
    if (!containerEl || !active) return null;
    const rect = containerEl.getBoundingClientRect();
    const widthPx = Math.round(rect.width);
    const heightPx = Math.round(rect.height);
    if (widthPx <= 0 || heightPx <= 0) return null;
    return { widthPx, heightPx, devicePixelRatio: window.devicePixelRatio || 1 };
  }

  function reportViewport() {
    const dims = measureViewport();
    if (!dims) return;
    const key = `${dims.widthPx}x${dims.heightPx}@${dims.devicePixelRatio}`;
    if (key === lastViewport) return;
    lastViewport = key;
    postToIframe({ type: 'embed.viewport', ...dims });
    void reportEmbedViewport(session.sessionId, dims.widthPx, dims.heightPx, dims.devicePixelRatio);
  }

  function scheduleViewportReport() {
    if (refitRaf) cancelAnimationFrame(refitRaf);
    refitRaf = requestAnimationFrame(() => {
      refitRaf = 0;
      reportViewport();
    });
  }

  function focusIframe() {
    iframeEl?.focus();
    iframeEl?.contentWindow?.focus();
  }

  function bindResizeObserver(node: HTMLDivElement) {
    containerEl = node;
    resizeObserver?.disconnect();
    resizeObserver = new ResizeObserver(() => scheduleViewportReport());
    resizeObserver.observe(node);
    return {
      destroy() {
        resizeObserver?.disconnect();
        resizeObserver = null;
        containerEl = null;
      },
    };
  }

  $: if (active && session.embed) {
    scheduleViewportReport();
    focusIframe();
    postToIframe({ type: 'embed.resume' });
    void reportEmbedActivity(session.sessionId, true);
  } else if (!active && session.embed) {
    postToIframe({ type: 'embed.suspend' });
    void reportEmbedActivity(session.sessionId, false);
  }

  onDestroy(() => {
    if (refitRaf) cancelAnimationFrame(refitRaf);
    resizeObserver?.disconnect();
    hostPort?.close();
    hostPort = null;
  });
</script>

{#if session.embed}
  <div class="embed-panel" use:bindResizeObserver>
    <iframe
      bind:this={iframeEl}
      class="embed-frame"
      title="{session.connectionName} embed"
      src={iframeSrc(session.embed.uiUrl)}
      sandbox={sandboxAttr(session.embed.sandbox)}
      on:load={() => {
        setupIframeChannel();
        scheduleViewportReport();
      }}
    ></iframe>
  </div>
{/if}

<style>
  .embed-panel {
    display: flex;
    flex: 1;
    min-height: 0;
    min-width: 0;
    overflow: hidden;
  }

  .embed-frame {
    flex: 1;
    width: 100%;
    height: 100%;
    border: 0;
    background: #000;
  }
</style>
