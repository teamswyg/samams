import http from '../../../shared/api/http';
import { endpoints } from '../../../shared/api/endpoints';

export async function getSentinelStatusApi() {
  const { data } = await http.get(endpoints.sentinel.status);
  return data;
}

export async function getAlertsApi(filters = {}) {
  const { data } = await http.get(endpoints.sentinel.alerts, { params: filters });
  return data;
}

export async function getLogsApi(filters = {}) {
  const { data } = await http.get(endpoints.sentinel.logs, { params: filters });
  return data;
}
