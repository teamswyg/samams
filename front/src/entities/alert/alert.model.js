export function toAlertViewModel(raw) {
  return {
    id: raw.id,
    eventName: raw.event_name || raw.eventName,
    severity: raw.severity,
    message: raw.message,
    occurredAt: raw.occurred_at || raw.occurredAt,
    subjectId: raw.subject_id || raw.subjectId,
  };
}
