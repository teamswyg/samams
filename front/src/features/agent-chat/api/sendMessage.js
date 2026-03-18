import http from '../../../shared/api/http';
import { endpoints } from '../../../shared/api/endpoints';

export async function sendMessageApi(sessionId, message) {
  const { data } = await http.post(endpoints.sessions.send(sessionId), { content: message });
  return data;
}
