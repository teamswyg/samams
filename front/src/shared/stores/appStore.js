import { create } from 'zustand';

export const useAppStore = create((set) => ({
  initialized: false,
  setInitialized: (v) => set({ initialized: v }),
}));
