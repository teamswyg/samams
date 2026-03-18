import axios from 'axios';
import { endpoints } from './endpoints';

const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || 'http://localhost:3000',
  timeout: 300000, // 5 min — AI calls (plan generation, tree conversion) can take 2-3 min
  headers: { 'Content-Type': 'application/json' },
});

// Auth header injection
http.interceptors.request.use((config) => {
  const token = sessionStorage.getItem('access_token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// --- Token refresh state ---
let isRefreshing = false;
let failedQueue = [];

function processQueue(error, token = null) {
  failedQueue.forEach(({ resolve, reject }) => {
    if (error) {
      reject(error);
    } else {
      resolve(token);
    }
  });
  failedQueue = [];
}

// Unwrap server envelope + auto-refresh on 401
http.interceptors.response.use(
  (res) => {
    if (res.data?.ok && res.data?.data !== undefined) {
      return { ...res, data: res.data.data };
    }
    return res;
  },
  async (error) => {
    const originalRequest = error.config;

    // 401 + not already retrying → try refresh
    if (error.response?.status === 401 && !originalRequest._retry) {
      const refreshToken = sessionStorage.getItem('refresh_token');
      if (!refreshToken) {
        sessionStorage.clear();
        window.location.href = '/login';
        return Promise.reject(error);
      }

      if (isRefreshing) {
        // Queue this request until refresh completes
        return new Promise((resolve, reject) => {
          failedQueue.push({ resolve, reject });
        }).then((token) => {
          originalRequest.headers.Authorization = `Bearer ${token}`;
          return http(originalRequest);
        });
      }

      originalRequest._retry = true;
      isRefreshing = true;

      try {
        // Call refresh endpoint with raw axios (skip interceptors)
        const { data: res } = await axios.post(
          (import.meta.env.VITE_API_BASE_URL || 'http://localhost:3000') + endpoints.auth.refresh,
          { refresh_token: refreshToken },
          { headers: { 'Content-Type': 'application/json' } }
        );

        // Server returns { ok: true, data: { access_token, ... } }
        // `res` is already the response body (via destructuring `data: res`)
        const newData = res.ok ? res.data : res;
        sessionStorage.setItem('access_token', newData.access_token);
        sessionStorage.setItem('refresh_token', newData.refresh_token);
        sessionStorage.setItem('token_expires_at', String(newData.expires_at));

        processQueue(null, newData.access_token);

        originalRequest.headers.Authorization = `Bearer ${newData.access_token}`;
        return http(originalRequest);
      } catch (refreshErr) {
        processQueue(refreshErr, null);
        sessionStorage.clear();
        window.location.href = '/login';
        return Promise.reject(refreshErr);
      } finally {
        isRefreshing = false;
      }
    }

    // Non-401 errors: normalize and reject
    const normalized = {
      status: error.response?.status || 0,
      message: error.response?.data?.error || error.message,
      raw: error,
    };
    return Promise.reject(normalized);
  }
);

export default http;
