import { create } from 'zustand';

export const useSessionStore = create((set) => ({
  activeSessionId: null,
  selectedRunId: null,
  streamStatus: 'idle',

  setActiveSessionId: (id) => set({ activeSessionId: id }),
  setSelectedRunId: (id) => set({ selectedRunId: id }),
  setStreamStatus: (status) => set({ streamStatus: status }),
}));
