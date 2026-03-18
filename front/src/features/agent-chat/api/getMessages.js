import http from '../../../shared/api/http';
import { endpoints } from '../../../shared/api/endpoints';

export async function getMessagesApi(sessionId) {
  const { data } = await http.get(endpoints.sessions.messages(sessionId));
  return data;
}
