import { create } from 'zustand';
import http from '../../../shared/api/http';
import { endpoints } from '../../../shared/api/endpoints';

export const useLogViewerStore = create((set, get) => ({
  logs: [],
  selectedLogId: null,
  isLoading: true,
  filters: {
    agent: 'ALL',
    eventType: 'ALL',
    dateRange: 'ALL',
  },
  eventTypes: ['ALL'],
  agentNames: ['ALL'],

  // Load logs from server.
  loadLogs: async () => {
    try {
      const { data } = await http.get(endpoints.run.logs);
      const serverLogs = data.logs || [];

      // Map server LogEntry format to log viewer format.
      const logs = serverLogs.map((entry, i) => ({
        id: entry.id || `log-${i}`,
        timestamp: entry.time || '',
        agent: entry.agent || 'System',
        agentId: entry.agent || '',
        eventType: entry.type || 'INFO',
        taskUid: '',
        description: entry.message || '',
        status: entry.type === 'ERROR' ? 'error' : entry.type === 'WARN' ? 'warning' : 'active',
      }));

      // Derive dynamic filter options from actual data.
      const agents = new Set(logs.map((l) => l.agent));
      const types = new Set(logs.map((l) => l.eventType));

      set({
        logs,
        isLoading: false,
        agentNames: ['ALL', ...agents],
        eventTypes: ['ALL', ...types],
      });
    } catch (err) {
      console.error('[logViewer] Failed to load logs:', err);
      set({ isLoading: false });
    }
  },

  selectLog: (id) => set({ selectedLogId: id }),

  setFilter: (key, value) => set((s) => ({
    filters: { ...s.filters, [key]: value },
  })),

  resetFilters: () => set({
    filters: { agent: 'ALL', eventType: 'ALL', dateRange: 'ALL' },
  }),

  getFilteredLogs: () => {
    const { logs, filters } = get();
    return logs.filter((log) => {
      if (filters.agent !== 'ALL' && log.agent !== filters.agent) return false;
      if (filters.eventType !== 'ALL' && log.eventType !== filters.eventType) return false;
      return true;
    });
  },

  getSelectedLog: () => {
    const { logs, selectedLogId } = get();
    return logs.find((l) => l.id === selectedLogId) || null;
  },
}));
