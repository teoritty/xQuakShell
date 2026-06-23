<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { preparePluginViewPanel, relayPluginViewMessage, releasePluginViewPanel } from '../stores/api';

  export let pluginId: string;
  export let panelId: string;
  export let assetUrl: string;
  export let title: string;

  let iframeEl: HTMLIFrameElement;
  let hostPort: MessagePort | null = null;
  let relayToken = '';

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

  function setupIframeChannel() {
    const target = iframeEl?.contentWindow;
    if (!target || hostPort) return;

    const channel = new MessageChannel();
    hostPort = channel.port1;
    hostPort.onmessage = (event: MessageEvent) => {
      void handlePluginMessage(event.data);
    };

    // Sandboxed iframe without allow-same-origin has an opaque origin ("null").
    target.postMessage(
      {
        source: 'xquakshell-host-init',
        pluginId,
        panelId,
      },
      'null',
      [channel.port2]
    );
  }

  async function handlePluginMessage(data: unknown) {
    if (!data || typeof data !== 'object') return;
    const payload = data as {
      source?: string;
      pluginId?: string;
      panelId?: string;
      message?: Record<string, unknown>;
    };
    if (payload.source !== 'xquakshell-plugin-view') return;
    if (payload.pluginId !== pluginId || payload.panelId !== panelId) return;
    try {
      await relayPluginViewMessage(relayToken, payload.message ?? {});
    } catch (e) {
      console.error('relayPluginViewMessage', e);
    }
  }

  function forwardHostMessage(event: Event) {
    const detail = (event as CustomEvent).detail;
    if (!detail || detail.pluginId !== pluginId || detail.panelId !== panelId) return;
    if (!hostPort) return;
    hostPort.postMessage({
      source: 'xquakshell-host',
      pluginId,
      panelId,
      message: detail.message,
    });
  }

  onMount(async () => {
    try {
      relayToken = await preparePluginViewPanel(pluginId, panelId);
    } catch (e) {
      console.error('preparePluginViewPanel', e);
    }
    window.addEventListener('plugin-view-message', forwardHostMessage as EventListener);
  });

  onDestroy(() => {
    if (relayToken) {
      releasePluginViewPanel(relayToken);
    }
    window.removeEventListener('plugin-view-message', forwardHostMessage as EventListener);
    hostPort?.close();
    hostPort = null;
  });
</script>

<div class="plugin-webview-panel">
  <div class="panel-header">{title}</div>
  <iframe
    bind:this={iframeEl}
    class="plugin-frame"
    title={title}
    src={iframeSrc(assetUrl)}
    sandbox="allow-scripts"
    on:load={setupIframeChannel}
  ></iframe>
</div>

<style>
  .plugin-webview-panel {
    display: flex;
    flex-direction: column;
    border-top: 1px solid var(--border-color);
    min-height: 120px;
    max-height: 220px;
    flex-shrink: 0;
  }

  .panel-header {
    padding: 4px 8px;
    font-size: 11px;
    font-weight: 500;
    color: var(--text-secondary);
    border-bottom: 1px solid var(--border-color);
  }

  .plugin-frame {
    flex: 1;
    width: 100%;
    border: none;
    background: var(--bg-primary);
  }
</style>
