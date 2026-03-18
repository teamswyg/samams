import { create } from 'zustand';

export const useSentinelUiStore = create((set) => ({
  selectedAlertId: null,
  logLevelFilter: 'all',
  autoRefresh: true,

  setSelectedAlertId: (id) => set({ selectedAlertId: id }),
  setLogLevelFilter: (level) => set({ logLevelFilter: level }),
  toggleAutoRefresh: () => set((s) => ({ autoRefresh: !s.autoRefresh })),
}));
