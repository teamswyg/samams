import { create } from 'zustand';

export const useWorkspaceStore = create((set) => ({
  currentTab: 'dashboard',
  sidebarCollapsed: false,

  setCurrentTab: (tab) => set({ currentTab: tab }),
  toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
}));
