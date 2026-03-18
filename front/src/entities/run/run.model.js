export function toRunViewModel(raw) {
  return {
    id: raw.id,
    sessionId: raw.session_id || raw.sessionId,
    status: raw.status,
    startedAt: raw.started_at || raw.startedAt,
    finishedAt: raw.finished_at || raw.finishedAt,
  };
}
