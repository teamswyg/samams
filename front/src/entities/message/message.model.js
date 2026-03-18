export function toMessageViewModel(raw) {
  return {
    id: raw.id,
    role: raw.role,
    content: raw.content ?? '',
    createdAt: raw.created_at || raw.createdAt,
    status: raw.status,
  };
}
