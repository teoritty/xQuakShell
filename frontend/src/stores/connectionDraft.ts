import { writable, get } from 'svelte/store';

interface ConnectionDraftState {
  protocolFieldHistory: Record<string, Record<string, unknown>>;
}

function createConnectionDraftStore() {
  const { subscribe, set, update } = writable<ConnectionDraftState>({
    protocolFieldHistory: {},
  });

  return {
    subscribe,
    setProtocolFields: (protocolId: string, fields: Record<string, unknown>) => {
      update((state) => ({
        ...state,
        protocolFieldHistory: {
          ...state.protocolFieldHistory,
          [protocolId]: fields,
        },
      }));
    },
    getProtocolFields: (protocolId: string): Record<string, unknown> | undefined => {
      const state = get({ subscribe });
      return state.protocolFieldHistory[protocolId];
    },
    clearProtocolFields: (protocolId: string) => {
      update((state) => {
        const newHistory = { ...state.protocolFieldHistory };
        delete newHistory[protocolId];
        return { ...state, protocolFieldHistory: newHistory };
      });
    },
    clear: () => set({ protocolFieldHistory: {} }),
  };
}

export const connectionDraftStore = createConnectionDraftStore();
