import { writable } from 'svelte/store';

import { getPluginContributions, type PluginContributions } from './api';



export const pluginContributions = writable<PluginContributions>({

  commands: [],

  views: [],

  statusBar: [],

});



export const pluginRuntimeStates = writable<Record<string, string>>({});



export async function refreshPluginContributions(): Promise<void> {

  pluginContributions.set(await getPluginContributions());

}



export function initPluginContributionEvents(): void {

  const rt = (window as any).runtime;

  if (!rt?.EventsOn) return;



  rt.EventsOn('PluginContributionsChanged', () => {

    void refreshPluginContributions();

  });



  rt.EventsOn('PluginStateChanged', (data: { pluginId: string; state: string }) => {

    if (!data?.pluginId) return;

    pluginRuntimeStates.update((states) => ({ ...states, [data.pluginId]: data.state }));

  });



  void refreshPluginContributions();

}



export function dispatchPluginViewMessage(detail: {

  pluginId: string;

  panelId: string;

  message: unknown;

}): void {

  window.dispatchEvent(new CustomEvent('plugin-view-message', { detail }));

}



export function initPluginViewMessageEvents(): void {

  const rt = (window as any).runtime;

  if (!rt?.EventsOn) return;



  rt.EventsOn('PluginViewMessage', (data: { pluginId: string; panelId: string; message: unknown }) => {

    dispatchPluginViewMessage(data);

  });

}

