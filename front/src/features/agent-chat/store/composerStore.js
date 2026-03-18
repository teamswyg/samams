import { create } from 'zustand';

export const useComposerStore = create((set) => ({
  draft: '',
  model: 'default-agent',

  setDraft: (draft) => set({ draft }),
  clearDraft: () => set({ draft: '' }),
  setModel: (model) => set({ model }),
}));
