export const queryKeys = {
  auth: {
    me: ['auth', 'me'],
  },
  sessions: {
    all: ['sessions'],
    detail: (sessionId) => ['sessions', sessionId],
    messages: (sessionId) => ['sessions', sessionId, 'messages'],
    runs: (sessionId) => ['sessions', sessionId, 'runs'],
  },
  sentinel: {
    status: ['sentinel', 'status'],
    alerts: (filters) => ['sentinel', 'alerts', filters],
    logs: (filters) => ['sentinel', 'logs', filters],
  },
  jobs: {
    all: (filters) => ['jobs', filters],
    detail: (jobId) => ['jobs', jobId],
  },
  settings: {
    all: ['settings'],
  },
};
