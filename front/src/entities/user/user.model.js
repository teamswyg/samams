export function toUserViewModel(raw) {
  return {
    id: raw.id,
    email: raw.email,
    displayName: raw.display_name || raw.displayName,
    aiTokenKey: raw.ai_token_key || raw.aiTokenKey,
  };
}
