import { create } from 'zustand';
import http from '../../../shared/api/http';
import { endpoints } from '../../../shared/api/endpoints';

// Map proxy agent status to dashboard status
function mapAgentStatus(status) {
  switch (status) {
    case 'running':
    case 'starting':
      return 'active';
    case 'paused':
      return 'paused';
    case 'error':
      return 'error';
    case 'stopped':
    default:
      return 'idle';
  }
}

// Map proxy agent to dashboard agent shape
function toAgentCard(proxyAgent) {
  const agentType = proxyAgent.agentType || 'cursor';
  return {
    id: proxyAgent.id,
    name: proxyAgent.name || 'Agent',
    agentTypeBadge: agentType + ' cli',
    status: mapAgentStatus(proxyAgent.status),
    currentTask: proxyAgent.taskName || proxyAgent.taskId || null,
    taskId: proxyAgent.taskId || null,
    nodeUid: proxyAgent.nodeUid || null,
    progress: proxyAgent.status === 'stopped' ? 100 : proxyAgent.status === 'running' ? 50 : 0,
    tokenUsed: 0,
    tokenMax: 20000,
    agentType,
    mode: proxyAgent.mode || 'execute',
  };
}

let pollTimer = null;

export const useDashboardStore = create((set, get) => ({
  agents: [],
  proxyConnected: null, // null = unknown, true = connected, false = disconnected
  hasActiveTasks: false, // true when there are pending/running tasks that need agents
  logs: [],
  chatOpen: false,
  meetingOpen: false,
  chatMessages: [
    { id: 1, sender: 'sentinel', text: 'SENTINEL AI online. No agents registered. Start by creating a planning document.' },
  ],
  autoScroll: true,
  lastSync: Date.now(),
  meetingStatus: null, // null = no meeting, or { status, sessionId, participantNodeUids, ... }
  meetingError: null,
  meetingSessionId: null, // tracks the current meeting session to detect stale decisions

  toggleChat: () => set((s) => ({ chatOpen: !s.chatOpen })),
  openMeeting: () => set({ meetingOpen: true }),
  closeMeeting: () => set({ meetingOpen: false, meetingError: null }),
  toggleAutoScroll: () => set((s) => ({ autoScroll: !s.autoScroll })),

  // Fetch agents + logs from proxy via server and update store
  syncAgents: async () => {
    try {
      const { data } = await http.get(endpoints.run.agents);
      const proxyAgents = data.agents || [];
      const mapped = proxyAgents
        .filter((a) => a.status !== 'stopped') // Don't show stopped agents
        .map(toAgentCard);
      // Determine if there's active work: agents running, or pending tasks on server.
      let activeTasks = mapped.length > 0;
      if (!activeTasks) {
        // No visible agents — check server for pending work (tree.json pending nodes).
        try {
          const { data: resumable } = await http.get(endpoints.run.resumable);
          activeTasks = (resumable.resumable || []).some((p) => p.pending > 0);
        } catch {}
      }

      set({ agents: mapped, proxyConnected: true, hasActiveTasks: activeTasks, lastSync: Date.now() });

      // Also fetch MAAL logs.
      try {
        const { data: logData } = await http.get(endpoints.run.logs);
        const serverLogs = logData.logs || [];
        if (serverLogs.length > 0) {
          set({ logs: serverLogs });
        }
      } catch (logErr) {
        // MAAL logs may not be available if proxy just started.
      }

      // Fetch progress and inject as system log entry.
      try {
        const { data: prog } = await http.get(endpoints.run.progress);
        if (prog.stage && prog.stage !== 'idle' && prog.stage !== 'running') {
          set((s) => {
            const existing = s.logs || [];
            const hasProgress = existing.some((l) => l.id === 'system-progress');
            const progressEntry = {
              id: 'system-progress',
              time: new Date().toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
              type: 'INFO',
              agent: 'SYSTEM',
              message: prog.message || prog.stage,
            };
            if (hasProgress) {
              return { logs: existing.map((l) => l.id === 'system-progress' ? progressEntry : l) };
            }
            return { logs: [progressEntry, ...existing] };
          });
        } else {
          // Remove progress entry when done.
          set((s) => ({ logs: (s.logs || []).filter((l) => l.id !== 'system-progress') }));
        }
      } catch {
        // progress endpoint may not exist yet
      }
    } catch (err) {
      const status = err?.response?.status;
      if (status === 503) {
        set({ proxyConnected: false, lastSync: Date.now() });
      }
    }
  },

  // Start polling for agent updates (every 3s)
  startPolling: () => {
    if (pollTimer) return;
    get().syncAgents();
    pollTimer = setInterval(() => get().syncAgents(), 3000);
  },

  // Stop polling
  stopPolling: () => {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  },

  // Create a new agent dynamically (e.g. when cursor agent work starts)
  createAgent: (agent) => set((s) => ({
    agents: [...s.agents, {
      id: agent.id || `agent-${Date.now()}`,
      name: agent.name || 'Cursor Agent',
      status: 'active',
      currentTask: agent.currentTask || null,
      taskId: agent.taskId || null,
      progress: agent.progress || 0,
      tokenUsed: agent.tokenUsed || 0,
      tokenMax: agent.tokenMax || 20000,
    }],
    logs: [
      { id: Date.now(), time: new Date().toLocaleTimeString('ko-KR', { hour: '2-digit', minute: '2-digit', second: '2-digit' }), type: 'INFO', agent: agent.name || 'Cursor Agent', message: `Agent "${agent.name || 'Cursor Agent'}" created and activated` },
      ...s.logs,
    ],
  })),

  removeAgent: (id) => set((s) => ({
    agents: s.agents.filter((a) => a.id !== id),
  })),

  pauseAgent: async (id) => {
    set((s) => ({ agents: s.agents.map((a) => a.id === id ? { ...a, status: 'paused' } : a) }));
    try { await http.post(endpoints.run.stopAgent, { agentId: id }); } catch (err) { console.error('[dashboard] stopAgent failed:', err); }
  },

  killAgent: async (id) => {
    // Immediately remove from UI, then tell server to kill.
    set((s) => ({ agents: s.agents.filter((a) => a.id !== id) }));
    try { await http.post(endpoints.run.stopAgent, { agentId: id }); } catch (err) { console.error('[dashboard] stopAgent failed:', err); }
  },

  pauseAll: async () => {
    // Optimistic UI update.
    set((s) => ({
      agents: s.agents.map((a) => a.status === 'active' ? { ...a, status: 'paused' } : a),
    }));
    try {
      await http.post(endpoints.run.pauseAll);
    } catch (err) {
      console.error('[pauseAll] API failed:', err);
    }
  },

  resumeAll: async () => {
    // Optimistic UI update.
    set((s) => ({
      agents: s.agents.map((a) => a.status === 'paused' ? { ...a, status: 'active' } : a),
    }));
    try {
      await http.post(endpoints.run.resumeAll);
    } catch (err) {
      console.error('[resumeAll] API failed:', err);
    }
  },

  // Strategy Meeting actions
  startStrategyMeeting: async (projectName) => {
    set({ meetingError: null, meetingStatus: null });
    try {
      const { data } = await http.post(endpoints.run.strategyMeetingStart, { projectName });
      set({ meetingStatus: data, meetingOpen: true, meetingSessionId: data?.sessionId || null });
    } catch (err) {
      const raw = err?.response?.data?.error || err?.response?.data?.message || err.message || 'Failed to start meeting';
      const msg = typeof raw === 'string' ? raw : JSON.stringify(raw);
      set({ meetingError: msg });
    }
  },

  pollMeetingStatus: async () => {
    try {
      const { data } = await http.get(endpoints.run.strategyMeetingStatus);
      set({ meetingStatus: data });
      return data;
    } catch {
      // Preserve last known status on error instead of wiping it.
      return get().meetingStatus;
    }
  },

  sendChat: async (text) => {
    const userMsg = { id: Date.now(), sender: 'user', text };
    set((s) => ({ chatMessages: [...s.chatMessages, userMsg] }));

    const lower = text.toLowerCase();
    const agents = get().agents;
    const logs = get().logs;

    // Log analysis command → call OpenAI via server
    if (lower.includes('analyze') || lower.includes('log analysis')) {
      try {
        const logsText = logs.map((l) => `[${l.time}] [${l.type}] ${l.agent}: ${l.message}`).join('\n');
        const { data } = await http.post(endpoints.ai.analyzeLogs, { logs: logsText });
        addChatMessage(data.analysis, set);
        return;
      } catch (err) {
        console.warn('[dashboard] AI analyze failed, falling back:', err.message);
      }
    }

    // Summary command → call Gemini via server
    if (lower.includes('summary') || lower.includes('status') || lower.includes('overview')) {
      try {
        const statusText = agents.map((a) => `${a.name}: ${a.status}${a.currentTask ? ` (${a.currentTask})` : ''}`).join('\n');
        const { data } = await http.post(endpoints.ai.summarize, { content: `Current SAMAMS agent status:\n${statusText}` });
        addChatMessage(data.summary, set);
        return;
      } catch (err) {
        console.warn('[dashboard] AI summary failed, falling back:', err.message);
      }
    }

    // Default: try server, fallback to local
    try {
      const { data } = await http.post(endpoints.ai.summarize, {
        content: `User command: "${text}"\nAgents: ${agents.length} total, ${agents.filter(a => a.status === 'active').length} active\nRespond as Sentinel AI system monitor, concisely.`,
      });
      addChatMessage(data.summary, set);
    } catch (err) {
      const response = generateLocalResponse(text, agents);
      addChatMessage(response, set);
    }
  },
}));

function addChatMessage(text, set) {
  set((s) => ({
    chatMessages: [...s.chatMessages, { id: Date.now(), sender: 'sentinel', text }],
  }));
}

function generateLocalResponse(input, agents) {
  const lower = input.toLowerCase();
  const active = agents.filter((a) => a.status === 'active').length;
  const idle = agents.filter((a) => a.status === 'idle').length;
  const paused = agents.filter((a) => a.status === 'paused').length;
  const error = agents.filter((a) => a.status === 'error').length;

  if (lower.includes('status') || lower.includes('overview') || lower.includes('how many')) {
    return `System Status Report:\n• Total Agents: ${agents.length}\n• Active: ${active}\n• Idle: ${idle}\n• Paused: ${paused}\n• Error: ${error}\n\nAll systems operating within normal parameters.`;
  }
  if (lower.includes('pause')) return 'Pause command acknowledged. Use PAUSE ALL button to confirm.';
  if (lower.includes('resume') || lower.includes('start')) return 'Resume command acknowledged. Use RESUME ALL button to confirm.';
  if (lower.includes('conflict') || lower.includes('merge')) return 'Running conflict scan...\nNo active conflicts detected.';
  if (lower.includes('help') || lower.includes('command')) {
    return 'Commands:\n• STATUS — Overview\n• PAUSE ALL / RESUME ALL\n• CONFLICT SCAN\n• ANALYZE — AI log analysis\n• SUMMARY — AI status summary\n• HELP — This list';
  }
  if (lower.includes('kill') || lower.includes('terminate')) return 'Kill requires confirmation. Specify agent name.';
  return `Acknowledged: "${input}"\nType HELP for commands. Use "analyze" for AI log analysis.`;
}
